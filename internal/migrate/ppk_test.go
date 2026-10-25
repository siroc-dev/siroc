package migrate

import (
	"bytes"
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
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/ssh"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestPPKEd25519RoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubBlob := append(sshBytes([]byte("ssh-ed25519")), sshBytes(pub)...)
	privBlob := sshBytes(priv.Seed())
	file := ppkFile(t, 2, "ssh-ed25519", "none", "test", pubBlob, privBlob, nil)
	signer, err := ParsePrivateKey(file, "")
	if err != nil {
		t.Fatal(err)
	}
	want, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(signer.PublicKey().Marshal(), want.Marshal()) {
		t.Fatal("public key mismatch")
	}
	if _, err := ParsePrivateKey(strings.Replace(file, "Private-MAC: ", "Private-MAC: 00", 1), ""); err == nil {
		t.Fatal("tampered MAC was accepted")
	}
}

func TestPPKEd25519EncryptedV2(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubBlob := append(sshBytes([]byte("ssh-ed25519")), sshBytes(pub)...)
	plain := padBlock(sshBytes(priv.Seed()))
	pass := "secret-pass"
	block, err := aes.NewCipher(v2Key(pass))
	if err != nil {
		t.Fatal(err)
	}
	enc := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(enc, plain)
	file := ppkFile(t, 2, "ssh-ed25519", "aes256-cbc", "locked", pubBlob, enc, plain)
	signer, err := ParsePrivateKey(file, pass)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := ssh.NewPublicKey(pub)
	if !bytes.Equal(signer.PublicKey().Marshal(), want.Marshal()) {
		t.Fatal("public key mismatch")
	}
	if _, err := ParsePrivateKey(file, ""); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKey(file, "nope"); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatal(err)
	}
}

func TestPPKEd25519EncryptedV3(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubBlob := append(sshBytes([]byte("ssh-ed25519")), sshBytes(pub)...)
	plain := padBlock(sshBytes(priv.Seed()))
	pass := "v3-secret"
	salt := bytes.Repeat([]byte{0x11}, 16)
	sum := argon2.IDKey([]byte(pass), salt, 1, 8, 1, 80)
	block, err := aes.NewCipher(sum[:32])
	if err != nil {
		t.Fatal(err)
	}
	enc := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, sum[32:48]).CryptBlocks(enc, plain)
	body := ppkBody(3, "ssh-ed25519", "aes256-cbc", "v3", pubBlob, enc)
	body = strings.Replace(body, "Private-Lines:", "Key-Derivation: Argon2id\nArgon2-Memory: 8\nArgon2-Passes: 1\nArgon2-Parallelism: 1\nArgon2-Salt: "+hex.EncodeToString(salt)+"\nPrivate-Lines:", 1)
	mac := hmacSHA256(sum[48:80], "ssh-ed25519", "aes256-cbc", "v3", pubBlob, plain)
	file := body + hex.EncodeToString(mac) + "\n"
	signer, err := ParsePrivateKey(file, pass)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := ssh.NewPublicKey(pub)
	if !bytes.Equal(signer.PublicKey().Marshal(), want.Marshal()) {
		t.Fatal("public key mismatch")
	}
}

func TestPPKRSAAndECDSA(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaPub := append(sshBytes([]byte("ssh-rsa")), append(mpint(big.NewInt(int64(rsaKey.E))), mpint(rsaKey.N)...)...)
	rsaPriv := bytes.Join([][]byte{mpint(rsaKey.D), mpint(rsaKey.Primes[0]), mpint(rsaKey.Primes[1]), mpint(rsaKey.Precomputed.Qinv)}, nil)
	file := ppkFile(t, 2, "ssh-rsa", "none", "rsa", rsaPub, rsaPriv, nil)
	signer, err := ParsePrivateKey(file, "")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := ssh.NewPublicKey(&rsaKey.PublicKey)
	if !bytes.Equal(signer.PublicKey().Marshal(), want.Marshal()) {
		t.Fatal("rsa public key mismatch")
	}

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	point := elliptic.Marshal(elliptic.P256(), ecKey.X, ecKey.Y)
	ecPub := append(sshBytes([]byte("ecdsa-sha2-nistp256")), append(sshBytes([]byte("nistp256")), sshBytes(point)...)...)
	ecPriv := mpint(ecKey.D)
	file = ppkFile(t, 2, "ecdsa-sha2-nistp256", "none", "ec", ecPub, ecPriv, nil)
	signer, err = ParsePrivateKey(file, "")
	if err != nil {
		t.Fatal(err)
	}
	want, _ = ssh.NewPublicKey(&ecKey.PublicKey)
	if !bytes.Equal(signer.PublicKey().Marshal(), want.Marshal()) {
		t.Fatal("ecdsa public key mismatch")
	}
}

func TestPPKOpenSSHAndRejects(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "panel")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ParsePrivateKey(string(pem.EncodeToMemory(block)), "")
	if err != nil {
		t.Fatal(err)
	}
	if signer.PublicKey().Type() != ssh.KeyAlgoED25519 {
		t.Fatal(signer.PublicKey().Type())
	}
	locked, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "panel", []byte("openssh-pass"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKey(string(pem.EncodeToMemory(locked)), ""); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatal(err)
	}
	got, err := ParsePrivateKey(string(pem.EncodeToMemory(locked)), "openssh-pass")
	if err != nil {
		t.Fatal(err)
	}
	if got.PublicKey().Type() != ssh.KeyAlgoED25519 {
		t.Fatal(got.PublicKey().Type())
	}
	if _, err := ParsePrivateKey("PuTTY-User-Key-File-1: ssh-rsa\n", ""); err == nil {
		t.Fatal("version 1 accepted")
	}
	if _, err := ParsePrivateKey("PuTTY-User-Key-File-2: ssh-dss\nEncryption: none\n", ""); err == nil || !strings.Contains(err.Error(), "DSA") {
		t.Fatal(err)
	}
}

func TestDialSSHRequiresSecret(t *testing.T) {
	_, err := DialSSH(rpc.DAConn{Username: "root", Host: "127.0.0.1", Mode: "ssh"})
	if err == nil || !strings.Contains(err.Error(), "PPK") {
		t.Fatal(err)
	}
}

func ppkFile(t *testing.T, version int, alg, encryption, comment string, pub, stored []byte, macPriv []byte) string {
	t.Helper()
	if macPriv == nil {
		macPriv = stored
	}
	body := ppkBody(version, alg, encryption, comment, pub, stored)
	var mac []byte
	if version <= 2 {
		h := sha1.New()
		h.Write([]byte("putty-private-key-file-mac-key"))
		if encryption != "none" {
			pass := "secret-pass"
			h.Write([]byte(pass))
		}
		mac = hmacSHA1(h.Sum(nil), alg, encryption, comment, pub, macPriv)
	}
	return body + hex.EncodeToString(mac) + "\n"
}

func ppkBody(version int, alg, encryption, comment string, pub, priv []byte) string {
	pubN, pubText := b64Lines(pub)
	privN, privText := b64Lines(priv)
	return strings.Join([]string{
		"PuTTY-User-Key-File-" + itoa(version) + ": " + alg,
		"Encryption: " + encryption,
		"Comment: " + comment,
		"Public-Lines: " + itoa(pubN),
		pubText,
		"Private-Lines: " + itoa(privN),
		privText,
		"Private-MAC: ",
	}, "\n")
}

func hmacSHA1(key []byte, alg, encryption, comment string, pub, priv []byte) []byte {
	m := hmac.New(sha1.New, key)
	m.Write(macMaterial(alg, encryption, comment, pub, priv))
	return m.Sum(nil)
}

func hmacSHA256(key []byte, alg, encryption, comment string, pub, priv []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(macMaterial(alg, encryption, comment, pub, priv))
	return m.Sum(nil)
}

func macMaterial(alg, encryption, comment string, pub, priv []byte) []byte {
	var buf bytes.Buffer
	for _, part := range [][]byte{[]byte(alg), []byte(encryption), []byte(comment), pub, priv} {
		buf.Write(sshBytes(part))
	}
	return buf.Bytes()
}

func v2Key(pass string) []byte {
	var out []byte
	for seq := byte(0); len(out) < 32; seq++ {
		h := sha1.Sum(append([]byte{0, 0, 0, seq}, []byte(pass)...))
		out = append(out, h[:]...)
	}
	return out[:32]
}

func sshBytes(b []byte) []byte {
	out := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(out, uint32(len(b)))
	copy(out[4:], b)
	return out
}

func mpint(n *big.Int) []byte {
	b := n.Bytes()
	if len(b) > 0 && b[0]&0x80 != 0 {
		b = append([]byte{0}, b...)
	}
	return sshBytes(b)
}

func padBlock(b []byte) []byte {
	n := aes.BlockSize - len(b)%aes.BlockSize
	if n == 0 {
		n = aes.BlockSize
	}
	return append(b, make([]byte, n)...)
}

func b64Lines(b []byte) (int, string) {
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
	if len(lines) == 0 {
		lines = []string{""}
	}
	return len(lines), strings.Join(lines, "\n")
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
