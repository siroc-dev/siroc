package rclone

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/ssh"
)

const maxPrivateKeyBytes = 256 << 10

// NormalizePrivateKey accepts an OpenSSH, PEM, PKCS#8, or PuTTY PPK private
// key and returns an unencrypted PKCS#8 PEM that rclone can read.
func NormalizePrivateKey(raw []byte, passphrase string) ([]byte, error) {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, fmt.Errorf("private key is empty")
	}
	if len(raw) > maxPrivateKeyBytes {
		return nil, fmt.Errorf("private key is too large")
	}
	var key crypto.PrivateKey
	var err error
	if strings.HasPrefix(string(raw), "PuTTY-User-Key-File-") {
		key, err = parsePPK(raw, passphrase)
	} else {
		key, err = parseOpenSSH(raw, passphrase)
	}
	if err != nil {
		return nil, err
	}
	return marshalPKCS8(key)
}

func parseOpenSSH(raw []byte, passphrase string) (crypto.PrivateKey, error) {
	key, err := ssh.ParseRawPrivateKey(raw)
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) || strings.Contains(strings.ToLower(err.Error()), "passphrase") || strings.Contains(strings.ToLower(err.Error()), "encrypted") {
			if passphrase == "" {
				return nil, fmt.Errorf("this key is encrypted; enter its passphrase")
			}
			key, err = ssh.ParseRawPrivateKeyWithPassphrase(raw, []byte(passphrase))
			if err != nil {
				return nil, fmt.Errorf("wrong passphrase")
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("unrecognized private key")
	}
	priv, ok := key.(crypto.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unsupported private key type")
	}
	return priv, nil
}

func marshalPKCS8(key crypto.PrivateKey) ([]byte, error) {
	switch k := key.(type) {
	case *rsa.PrivateKey, *ecdsa.PrivateKey, ed25519.PrivateKey:
	case *ed25519.PrivateKey:
		key = *k
	default:
		return nil, fmt.Errorf("unsupported private key type %T", key)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("unsupported private key type")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func parsePPK(raw []byte, passphrase string) (crypto.PrivateKey, error) {
	lines := splitLines(string(raw))
	if len(lines) == 0 {
		return nil, fmt.Errorf("not a PuTTY private key")
	}
	header, value, ok := splitHeader(lines[0])
	if !ok {
		return nil, fmt.Errorf("not a PuTTY private key")
	}
	version := 0
	switch header {
	case "PuTTY-User-Key-File-2":
		version = 2
	case "PuTTY-User-Key-File-3":
		version = 3
	case "PuTTY-User-Key-File-1":
		return nil, fmt.Errorf("PuTTY key format 1 is not supported")
	default:
		if strings.HasPrefix(header, "PuTTY-User-Key-File-") {
			return nil, fmt.Errorf("PuTTY key format is too new")
		}
		return nil, fmt.Errorf("not a PuTTY private key")
	}
	algo := value
	i := 1
	headers := map[string]string{}
	for i < len(lines) {
		h, v, ok := splitHeader(lines[i])
		if !ok || h == "Public-Lines" {
			break
		}
		headers[h] = v
		i++
	}
	if i >= len(lines) {
		return nil, fmt.Errorf("private key file is damaged")
	}
	_, nlines, ok := splitHeader(lines[i])
	if !ok || !strings.HasPrefix(lines[i], "Public-Lines:") {
		return nil, fmt.Errorf("private key file is damaged")
	}
	pubCount, err := atoiLines(nlines)
	if err != nil {
		return nil, err
	}
	i++
	public, i, err := readB64(lines, i, pubCount)
	if err != nil {
		return nil, err
	}
	for i < len(lines) {
		h, v, ok := splitHeader(lines[i])
		if !ok || h == "Private-Lines" {
			break
		}
		headers[h] = v
		i++
	}
	if i >= len(lines) || !strings.HasPrefix(lines[i], "Private-Lines:") {
		return nil, fmt.Errorf("private key file is damaged")
	}
	_, nlines, _ = splitHeader(lines[i])
	privCount, err := atoiLines(nlines)
	if err != nil {
		return nil, err
	}
	i++
	private, i, err := readB64(lines, i, privCount)
	if err != nil {
		return nil, err
	}
	macHex := ""
	for i < len(lines) {
		if lines[i] == "" {
			i++
			continue
		}
		h, v, ok := splitHeader(lines[i])
		if ok && (h == "Private-MAC" || h == "Private-Hash") {
			macHex = v
		}
		i++
	}
	if macHex == "" {
		return nil, fmt.Errorf("private key file is damaged")
	}
	encryption := headers["Encryption"]
	comment := headers["Comment"]
	if encryption == "" {
		return nil, fmt.Errorf("private key file is damaged")
	}
	encrypted := encryption != "none"
	if encrypted && encryption != "aes256-cbc" {
		return nil, fmt.Errorf("unsupported PuTTY encryption %s", encryption)
	}
	if encrypted && passphrase == "" {
		return nil, fmt.Errorf("this key is encrypted; enter its passphrase")
	}
	plain := private
	macKey := []byte{}
	if encrypted {
		var cipherKey, iv []byte
		var err error
		cipherKey, iv, macKey, err = ppkKeys(version, passphrase, headers)
		if err != nil {
			return nil, err
		}
		plain, err = decryptAES256CBC(cipherKey, iv, private)
		if err != nil {
			return nil, err
		}
	} else if version == 2 {
		sum := sha1.Sum([]byte("putty-private-key-file-mac-key"))
		macKey = sum[:]
	}
	if err := checkPPKMAC(version, macKey, macHex, algo, encryption, comment, public, plain); err != nil {
		if encrypted {
			return nil, fmt.Errorf("wrong passphrase")
		}
		return nil, err
	}
	return ppkPrivate(algo, public, plain)
}

func ppkKeys(version int, passphrase string, headers map[string]string) (key, iv, mac []byte, err error) {
	if version < 3 {
		sum := sha1.Sum([]byte("putty-private-key-file-mac-key" + passphrase))
		mac = sum[:]
		buf := make([]byte, 0, 40)
		for seq := uint32(0); len(buf) < 32; seq++ {
			h := sha1.New()
			var n [4]byte
			binary.BigEndian.PutUint32(n[:], seq)
			h.Write(n[:])
			h.Write([]byte(passphrase))
			buf = h.Sum(buf)
		}
		return buf[:32], make([]byte, aes.BlockSize), mac, nil
	}
	flavour := headers["Key-Derivation"]
	mem, err := atoiLines(headers["Argon2-Memory"])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("private key file is damaged")
	}
	passes, err := atoiLines(headers["Argon2-Passes"])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("private key file is damaged")
	}
	par, err := atoiLines(headers["Argon2-Parallelism"])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("private key file is damaged")
	}
	salt, err := hex.DecodeString(headers["Argon2-Salt"])
	if err != nil || len(salt) == 0 {
		return nil, nil, nil, fmt.Errorf("private key file is damaged")
	}
	if mem < 8 || mem > 512*1024 || passes < 1 || passes > 256 || par < 1 || par > 8 {
		return nil, nil, nil, fmt.Errorf("unsupported key derivation parameters")
	}
	var out []byte
	switch flavour {
	case "Argon2id":
		out = argon2.IDKey([]byte(passphrase), salt, uint32(passes), uint32(mem), uint8(par), 80)
	case "Argon2i":
		out = argon2.Key([]byte(passphrase), salt, uint32(passes), uint32(mem), uint8(par), 80)
	default:
		return nil, nil, nil, fmt.Errorf("unsupported key derivation %s", flavour)
	}
	return out[:32], out[32:48], out[48:80], nil
}

func checkPPKMAC(version int, macKey []byte, macHex, algo, encryption, comment string, public, private []byte) error {
	want, err := hex.DecodeString(strings.TrimSpace(macHex))
	if err != nil {
		return fmt.Errorf("private key file is damaged")
	}
	var mac []byte
	data := macData(algo, encryption, comment, public, private)
	if version >= 3 {
		h := hmac.New(sha256.New, macKey)
		h.Write(data)
		mac = h.Sum(nil)
	} else {
		h := hmac.New(sha1.New, macKey)
		h.Write(data)
		mac = h.Sum(nil)
	}
	if !hmac.Equal(mac, want) {
		return fmt.Errorf("private key file is damaged")
	}
	return nil
}

func macData(algo, encryption, comment string, public, private []byte) []byte {
	var b strings.Builder
	writeSSHString(&b, []byte(algo))
	writeSSHString(&b, []byte(encryption))
	writeSSHString(&b, []byte(comment))
	writeSSHString(&b, public)
	writeSSHString(&b, private)
	return []byte(b.String())
}

func writeSSHString(b *strings.Builder, p []byte) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(p)))
	b.Write(n[:])
	b.Write(p)
}

func decryptAES256CBC(key, iv, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("private key file is damaged")
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("private key file is damaged")
	}
	out := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ciphertext)
	return out, nil
}

func ppkPrivate(algo string, public, private []byte) (crypto.PrivateKey, error) {
	pub := &sshReader{b: public}
	pubAlgo, err := pub.text()
	if err != nil || pubAlgo != algo {
		return nil, fmt.Errorf("private key file is damaged")
	}
	priv := &sshReader{b: private}
	switch algo {
	case "ssh-rsa":
		e, err := pub.mpint()
		if err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		n, err := pub.mpint()
		if err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		d, err := priv.mpint()
		if err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		p, err := priv.mpint()
		if err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		q, err := priv.mpint()
		if err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		if _, err := priv.mpint(); err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		if e.Sign() <= 0 || e.BitLen() > 32 {
			return nil, fmt.Errorf("private key file is damaged")
		}
		key := &rsa.PrivateKey{
			PublicKey: rsa.PublicKey{N: n, E: int(e.Int64())},
			D:         d,
			Primes:    []*big.Int{p, q},
		}
		if err := key.Validate(); err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		return key, nil
	case "ssh-ed25519":
		pubKey, err := pub.raw()
		if err != nil || len(pubKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("private key file is damaged")
		}
		seed, err := priv.mpintBytes()
		if err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		if len(seed) > ed25519.SeedSize {
			return nil, fmt.Errorf("private key file is damaged")
		}
		padded := make([]byte, ed25519.SeedSize)
		copy(padded[ed25519.SeedSize-len(seed):], seed)
		key := ed25519.NewKeyFromSeed(padded)
		if !key.Public().(ed25519.PublicKey).Equal(ed25519.PublicKey(pubKey)) {
			return nil, fmt.Errorf("private key file is damaged")
		}
		return key, nil
	case "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		curveName, err := pub.text()
		if err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		point, err := pub.raw()
		if err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		d, err := priv.mpint()
		if err != nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		var curve elliptic.Curve
		switch curveName {
		case "nistp256":
			curve = elliptic.P256()
		case "nistp384":
			curve = elliptic.P384()
		case "nistp521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("unsupported private key type")
		}
		x, y := elliptic.Unmarshal(curve, point)
		if x == nil {
			return nil, fmt.Errorf("private key file is damaged")
		}
		key := &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y}, D: d}
		if !key.PublicKey.Curve.IsOnCurve(key.X, key.Y) {
			return nil, fmt.Errorf("private key file is damaged")
		}
		return key, nil
	default:
		return nil, fmt.Errorf("unsupported private key type")
	}
}

type sshReader struct {
	b []byte
}

func (r *sshReader) take(n int) ([]byte, error) {
	if n < 0 || len(r.b) < n {
		return nil, fmt.Errorf("short")
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out, nil
}

func (r *sshReader) raw() ([]byte, error) {
	if len(r.b) < 4 {
		return nil, fmt.Errorf("short")
	}
	n := int(binary.BigEndian.Uint32(r.b[:4]))
	r.b = r.b[4:]
	return r.take(n)
}

func (r *sshReader) text() (string, error) {
	b, err := r.raw()
	return string(b), err
}

func (r *sshReader) mpint() (*big.Int, error) {
	b, err := r.raw()
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(b), nil
}

func (r *sshReader) mpintBytes() ([]byte, error) {
	b, err := r.raw()
	if err != nil {
		return nil, err
	}
	for len(b) > 0 && b[0] == 0 {
		b = b[1:]
	}
	return b, nil
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	parts := strings.Split(s, "\n")
	out := parts[:0]
	for _, line := range parts {
		if line == "" && len(out) > 0 {
			continue
		}
		out = append(out, line)
	}
	return out
}

func splitHeader(line string) (string, string, bool) {
	name, value, ok := strings.Cut(line, ": ")
	if !ok || name == "" || strings.Contains(name, " ") {
		return "", "", false
	}
	return name, value, true
}

func atoiLines(s string) (int, error) {
	s = strings.TrimSpace(s)
	n := 0
	if s == "" {
		return 0, fmt.Errorf("private key file is damaged")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("private key file is damaged")
		}
		n = n*10 + int(c-'0')
		if n > 10000 {
			return 0, fmt.Errorf("private key file is damaged")
		}
	}
	return n, nil
}

func readB64(lines []string, i, n int) ([]byte, int, error) {
	if n < 0 || i+n > len(lines) {
		return nil, i, fmt.Errorf("private key file is damaged")
	}
	blob, err := base64.StdEncoding.DecodeString(strings.Join(lines[i:i+n], ""))
	if err != nil {
		return nil, i, fmt.Errorf("private key file is damaged")
	}
	return blob, i + n, nil
}
