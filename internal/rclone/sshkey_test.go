package rclone

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/ssh"
)

func TestNormalizeOpenSSH(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatal(err)
	}
	out, err := NormalizePrivateKey(pem.EncodeToMemory(block), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "BEGIN PRIVATE KEY") {
		t.Fatalf("not pkcs8: %s", out)
	}
	got := parseBack(t, out)
	if !got.(ed25519.PrivateKey).Equal(priv) {
		t.Fatal("ed25519 key changed")
	}

	locked, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "test", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizePrivateKey(pem.EncodeToMemory(locked), ""); err == nil {
		t.Fatal("encrypted key accepted without a passphrase")
	}
	out, err = NormalizePrivateKey(pem.EncodeToMemory(locked), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !parseBack(t, out).(ed25519.PrivateKey).Equal(priv) {
		t.Fatal("unlocked ed25519 key changed")
	}
}

func TestNormalizePPK(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	body := ppk(t, 3, "ssh-ed25519", "ed", sshEdPub(pub), sshMpint(priv.Seed()), "")
	out, err := NormalizePrivateKey([]byte(body), "")
	if err != nil {
		t.Fatal(err)
	}
	if !parseBack(t, out).(ed25519.PrivateKey).Equal(priv) {
		t.Fatal("ppk ed25519 changed")
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaBody := ppk(t, 2, "ssh-rsa", "rsa", sshRSAPub(rsaKey), sshRSAPriv(rsaKey), "secret")
	if _, err := NormalizePrivateKey([]byte(rsaBody), ""); err == nil {
		t.Fatal("encrypted ppk accepted without a passphrase")
	}
	if _, err := NormalizePrivateKey([]byte(rsaBody), "nope"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	out, err = NormalizePrivateKey([]byte(rsaBody), "secret")
	if err != nil {
		t.Fatal(err)
	}
	got := parseBack(t, out).(*rsa.PrivateKey)
	if got.N.Cmp(rsaKey.N) != 0 || got.E != rsaKey.E || got.D.Cmp(rsaKey.D) != 0 {
		t.Fatal("ppk rsa changed")
	}

	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecBody := ppk(t, 2, "ecdsa-sha2-nistp256", "ec", sshECPub("nistp256", ec), sshMpint(ec.D.Bytes()), "")
	out, err = NormalizePrivateKey([]byte(ecBody), "")
	if err != nil {
		t.Fatal(err)
	}
	gotEC := parseBack(t, out).(*ecdsa.PrivateKey)
	if gotEC.D.Cmp(ec.D) != 0 {
		t.Fatal("ppk ecdsa changed")
	}
}

func parseBack(t *testing.T, pemBytes []byte) any {
	t.Helper()
	key, err := ssh.ParseRawPrivateKey(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func ppk(t *testing.T, version int, algo, comment string, public, private []byte, pass string) string {
	t.Helper()
	plain := append([]byte(nil), private...)
	blob := plain
	encryption := "none"
	var extra string
	macKey := []byte{}
	if pass != "" {
		encryption = "aes256-cbc"
		for len(plain)%aes.BlockSize != 0 {
			plain = append(plain, 7)
		}
		var key, iv []byte
		if version >= 3 {
			salt := []byte("salt-for-test!!")
			extra = "Key-Derivation: Argon2id\nArgon2-Memory: 8\nArgon2-Passes: 1\nArgon2-Parallelism: 1\nArgon2-Salt: " + hex.EncodeToString(salt) + "\n"
			out := argon2.IDKey([]byte(pass), salt, 1, 8, 1, 80)
			key, iv, macKey = out[:32], out[32:48], out[48:80]
		} else {
			sum := sha1.Sum([]byte("putty-private-key-file-mac-key" + pass))
			macKey = sum[:]
			buf := make([]byte, 0, 40)
			for seq := uint32(0); len(buf) < 32; seq++ {
				h := sha1.New()
				var n [4]byte
				binary.BigEndian.PutUint32(n[:], seq)
				h.Write(n[:])
				h.Write([]byte(pass))
				buf = h.Sum(buf)
			}
			key = buf[:32]
			iv = make([]byte, aes.BlockSize)
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			t.Fatal(err)
		}
		blob = make([]byte, len(plain))
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(blob, plain)
	} else if version == 2 {
		sum := sha1.Sum([]byte("putty-private-key-file-mac-key"))
		macKey = sum[:]
	}
	data := macData(algo, encryption, comment, public, plain)
	var mac []byte
	if version >= 3 {
		h := hmac.New(sha256.New, macKey)
		h.Write(data)
		mac = h.Sum(nil)
	} else {
		h := hmac.New(sha1.New, macKey)
		h.Write(data)
		mac = h.Sum(nil)
	}
	return strings.Join([]string{
		"PuTTY-User-Key-File-" + itoa(version) + ": " + algo,
		"Encryption: " + encryption,
		"Comment: " + comment,
		"Public-Lines: " + itoa(b64Lines(public)),
		wrap64(public),
		extra + "Private-Lines: " + itoa(b64Lines(blob)),
		wrap64(blob),
		"Private-MAC: " + hex.EncodeToString(mac),
		"",
	}, "\n")
}

func b64Lines(b []byte) int {
	s := base64.StdEncoding.EncodeToString(b)
	n := 0
	for len(s) > 0 {
		n++
		if len(s) > 64 {
			s = s[64:]
		} else {
			break
		}
	}
	return n
}

func wrap64(b []byte) string {
	s := base64.StdEncoding.EncodeToString(b)
	var lines []string
	for len(s) > 0 {
		n := 64
		if n > len(s) {
			n = len(s)
		}
		lines = append(lines, s[:n])
		s = s[n:]
	}
	return strings.Join(lines, "\n")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

func sshString(s string) []byte {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	return append(n[:], s...)
}

func sshMpint(b []byte) []byte {
	if len(b) == 0 {
		b = []byte{0}
	}
	if b[0]&0x80 != 0 {
		b = append([]byte{0}, b...)
	}
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	return append(n[:], b...)
}

func sshEdPub(pub ed25519.PublicKey) []byte {
	out := append(sshString("ssh-ed25519"), sshString(string(pub))...)
	return out
}

func sshRSAPub(key *rsa.PrivateKey) []byte {
	e := key.E
	var eb []byte
	for e > 0 {
		eb = append([]byte{byte(e)}, eb...)
		e >>= 8
	}
	return append(append(sshString("ssh-rsa"), sshMpint(eb)...), sshMpint(key.N.Bytes())...)
}

func sshRSAPriv(key *rsa.PrivateKey) []byte {
	out := sshMpint(key.D.Bytes())
	out = append(out, sshMpint(key.Primes[0].Bytes())...)
	out = append(out, sshMpint(key.Primes[1].Bytes())...)
	return append(out, sshMpint(key.Precomputed.Qinv.Bytes())...)
}

func sshECPub(name string, key *ecdsa.PrivateKey) []byte {
	point := elliptic.Marshal(key.Curve, key.X, key.Y)
	out := sshString("ecdsa-sha2-" + name)
	out = append(out, sshString(name)...)
	return append(out, sshString(string(point))...)
}

func TestMarshalPKCS8RejectsUnknown(t *testing.T) {
	if _, err := NormalizePrivateKey([]byte("not a key"), ""); err == nil {
		t.Fatal("garbage accepted")
	}
	der, err := x509.MarshalPKCS8PrivateKey(mustRSA(t))
	if err != nil {
		t.Fatal(err)
	}
	out, err := NormalizePrivateKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "-----BEGIN PRIVATE KEY-----") {
		t.Fatal(string(out))
	}
}

func mustRSA(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
