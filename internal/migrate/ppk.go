package migrate

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"math/big"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/ssh"
)

const maxPrivateKeyBytes = 256 * 1024

// ParsePrivateKey reads a PuTTY .ppk file or an OpenSSH/PEM private key.
// passphrase is only used when the key is encrypted.
func ParsePrivateKey(text, passphrase string) (ssh.Signer, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("private key is empty")
	}
	if len(text) > maxPrivateKeyBytes {
		return nil, fmt.Errorf("private key file is too large")
	}
	if strings.Contains(text, "PuTTY-User-Key-File-") {
		return parsePPK(text, passphrase)
	}
	var (
		key any
		err error
	)
	raw := []byte(text)
	if passphrase != "" {
		key, err = ssh.ParseRawPrivateKeyWithPassphrase(raw, []byte(passphrase))
	} else {
		key, err = ssh.ParseRawPrivateKey(raw)
	}
	if err != nil {
		return nil, privateKeyError(err, passphrase != "")
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, fmt.Errorf("could not read the private key")
	}
	return signer, nil
}

func privateKeyError(err error, hadPassphrase bool) error {
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "passphrase") || strings.Contains(low, "encrypted") || strings.Contains(low, "decrypt") {
		if hadPassphrase {
			return fmt.Errorf("wrong key passphrase")
		}
		return fmt.Errorf("this key is encrypted; enter its passphrase")
	}
	return fmt.Errorf("could not read the private key")
}

func parsePPK(text, passphrase string) (ssh.Signer, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	p := &ppkParser{lines: strings.Split(text, "\n")}
	header, alg, err := p.header()
	if err != nil {
		return nil, err
	}
	version := 0
	switch header {
	case "PuTTY-User-Key-File-2":
		version = 2
	case "PuTTY-User-Key-File-3":
		version = 3
	case "PuTTY-User-Key-File-1":
		return nil, fmt.Errorf("this PPK is an old version 1 key. Save it again from PuTTY")
	default:
		return nil, fmt.Errorf("not a PuTTY private key")
	}
	switch alg {
	case "ssh-rsa", "ssh-ed25519", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
	case "ssh-dss":
		return nil, fmt.Errorf("this PPK uses DSA, which is not supported. Save the key again as Ed25519 or RSA")
	default:
		return nil, fmt.Errorf("this PPK uses %s, which is not supported", alg)
	}
	name, encryption, err := p.header()
	if err != nil || name != "Encryption" {
		return nil, fmt.Errorf("invalid PPK file")
	}
	if encryption != "none" && encryption != "aes256-cbc" {
		return nil, fmt.Errorf("this PPK uses %s encryption, which is not supported", encryption)
	}
	name, comment, err := p.header()
	if err != nil || name != "Comment" {
		return nil, fmt.Errorf("invalid PPK file")
	}
	pub, err := p.blob("Public-Lines")
	if err != nil {
		return nil, err
	}
	var (
		kdf     string
		memory  uint32
		passes  uint32
		threads uint8
		salt    []byte
	)
	if version >= 3 && encryption != "none" {
		kdf, memory, passes, threads, salt, err = p.argon2()
		if err != nil {
			return nil, err
		}
	}
	priv, err := p.blob("Private-Lines")
	if err != nil {
		return nil, err
	}
	name, macText, err := p.header()
	if err != nil || name != "Private-MAC" {
		return nil, fmt.Errorf("invalid PPK file")
	}
	wantMAC, err := hex.DecodeString(strings.TrimSpace(macText))
	if err != nil {
		return nil, fmt.Errorf("invalid PPK file")
	}
	encrypted := encryption == "aes256-cbc"
	if encrypted && passphrase == "" {
		return nil, fmt.Errorf("this key is encrypted; enter its passphrase")
	}
	macKey, plain, err := unlockPPK(version, encrypted, passphrase, kdf, salt, passes, memory, threads, priv)
	if err != nil {
		return nil, err
	}
	sum := ppkMAC(version, macKey, alg, encryption, comment, pub, plain)
	if !hmac.Equal(sum, wantMAC) {
		if encrypted {
			return nil, fmt.Errorf("wrong key passphrase")
		}
		return nil, fmt.Errorf("private key check failed")
	}
	raw, err := privateFromPPK(alg, pub, plain, encrypted)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		return nil, fmt.Errorf("could not read the private key")
	}
	return signer, nil
}

type ppkParser struct {
	lines []string
	i     int
}

func (p *ppkParser) header() (string, string, error) {
	for p.i < len(p.lines) && strings.TrimSpace(p.lines[p.i]) == "" {
		p.i++
	}
	if p.i >= len(p.lines) {
		return "", "", fmt.Errorf("invalid PPK file")
	}
	line := p.lines[p.i]
	p.i++
	key, val, ok := strings.Cut(line, ": ")
	if !ok {
		if strings.HasSuffix(line, ":") {
			return strings.TrimSpace(strings.TrimSuffix(line, ":")), "", nil
		}
		return "", "", fmt.Errorf("invalid PPK file")
	}
	return strings.TrimSpace(key), val, nil
}

func (p *ppkParser) blob(header string) ([]byte, error) {
	name, countText, err := p.header()
	if err != nil || name != header {
		return nil, fmt.Errorf("invalid PPK file")
	}
	n, err := parseCount(countText)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for j := 0; j < n; j++ {
		if p.i >= len(p.lines) {
			return nil, fmt.Errorf("invalid PPK file")
		}
		b.WriteString(strings.TrimSpace(p.lines[p.i]))
		p.i++
	}
	raw, err := base64.StdEncoding.DecodeString(b.String())
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(b.String())
	}
	if err != nil {
		return nil, fmt.Errorf("invalid PPK file")
	}
	return raw, nil
}

func parseCount(text string) (int, error) {
	n := 0
	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("invalid PPK file")
	}
	for _, r := range strings.TrimSpace(text) {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid PPK file")
		}
		n = n*10 + int(r-'0')
		if n > 512 {
			return 0, fmt.Errorf("invalid PPK file")
		}
	}
	return n, nil
}

func (p *ppkParser) argon2() (string, uint32, uint32, uint8, []byte, error) {
	name, kdf, err := p.header()
	if err != nil || name != "Key-Derivation" {
		return "", 0, 0, 0, nil, fmt.Errorf("invalid PPK file")
	}
	if kdf != "Argon2id" && kdf != "Argon2i" {
		return "", 0, 0, 0, nil, fmt.Errorf("this PPK uses %s, which is not supported. Save it again from PuTTY with Argon2id", kdf)
	}
	name, memText, err := p.header()
	if err != nil || name != "Argon2-Memory" {
		return "", 0, 0, 0, nil, fmt.Errorf("invalid PPK file")
	}
	name, passText, err := p.header()
	if err != nil || name != "Argon2-Passes" {
		return "", 0, 0, 0, nil, fmt.Errorf("invalid PPK file")
	}
	name, parText, err := p.header()
	if err != nil || name != "Argon2-Parallelism" {
		return "", 0, 0, 0, nil, fmt.Errorf("invalid PPK file")
	}
	name, saltText, err := p.header()
	if err != nil || name != "Argon2-Salt" {
		return "", 0, 0, 0, nil, fmt.Errorf("invalid PPK file")
	}
	memory, err := parseU32(memText, 8, 128*1024)
	if err != nil {
		return "", 0, 0, 0, nil, fmt.Errorf("this PPK asks for too much memory to unlock")
	}
	passes, err := parseU32(passText, 1, 64)
	if err != nil {
		return "", 0, 0, 0, nil, fmt.Errorf("this PPK asks for too much work to unlock")
	}
	parallel, err := parseU32(parText, 1, 16)
	if err != nil {
		return "", 0, 0, 0, nil, fmt.Errorf("invalid PPK file")
	}
	if memory < 8*parallel {
		return "", 0, 0, 0, nil, fmt.Errorf("invalid PPK file")
	}
	salt, err := hex.DecodeString(strings.TrimSpace(saltText))
	if err != nil || len(salt) == 0 || len(salt) > 256 {
		return "", 0, 0, 0, nil, fmt.Errorf("invalid PPK file")
	}
	return kdf, memory, passes, uint8(parallel), salt, nil
}

func parseU32(text string, min, max uint32) (uint32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("invalid")
	}
	var n uint32
	for _, r := range text {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid")
		}
		next := n*10 + uint32(r-'0')
		if next < n || next > max {
			return 0, fmt.Errorf("invalid")
		}
		n = next
	}
	if n < min {
		return 0, fmt.Errorf("invalid")
	}
	return n, nil
}

func unlockPPK(version int, encrypted bool, passphrase, kdf string, salt []byte, passes, memory uint32, threads uint8, priv []byte) ([]byte, []byte, error) {
	if version <= 2 {
		h := sha1.New()
		h.Write([]byte("putty-private-key-file-mac-key"))
		if encrypted {
			h.Write([]byte(passphrase))
		}
		macKey := h.Sum(nil)
		if !encrypted {
			return macKey, priv, nil
		}
		plain, err := decryptAES(v2CipherKey(passphrase), make([]byte, aes.BlockSize), priv)
		if err != nil {
			return nil, nil, err
		}
		return macKey, plain, nil
	}
	if !encrypted {
		return nil, priv, nil
	}
	sum, err := ppkArgon2(kdf, passphrase, salt, passes, memory, threads)
	if err != nil {
		return nil, nil, err
	}
	plain, err := decryptAES(sum[:32], sum[32:48], priv)
	if err != nil {
		return nil, nil, err
	}
	return sum[48:80], plain, nil
}

func v2CipherKey(passphrase string) []byte {
	var stretched []byte
	for seq := uint32(0); len(stretched) < 32; seq++ {
		h := sha1.New()
		var ctr [4]byte
		binary.BigEndian.PutUint32(ctr[:], seq)
		h.Write(ctr[:])
		h.Write([]byte(passphrase))
		stretched = append(stretched, h.Sum(nil)...)
	}
	return stretched[:32]
}

func decryptAES(key, iv, priv []byte) ([]byte, error) {
	if len(priv) == 0 || len(priv)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid PPK file")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("invalid PPK file")
	}
	plain := make([]byte, len(priv))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, priv)
	return plain, nil
}

func ppkArgon2(kdf, passphrase string, salt []byte, passes, memory uint32, threads uint8) ([]byte, error) {
	switch kdf {
	case "Argon2id":
		return argon2.IDKey([]byte(passphrase), salt, passes, memory, threads, 80), nil
	case "Argon2i":
		return argon2.Key([]byte(passphrase), salt, passes, memory, threads, 80), nil
	default:
		return nil, fmt.Errorf("invalid PPK file")
	}
}

func ppkMAC(version int, macKey []byte, alg, encryption, comment string, pub, priv []byte) []byte {
	var buf bytes.Buffer
	writeSSHString(&buf, []byte(alg))
	writeSSHString(&buf, []byte(encryption))
	writeSSHString(&buf, []byte(comment))
	writeSSHString(&buf, pub)
	writeSSHString(&buf, priv)
	var mac hash.Hash
	if version <= 2 {
		mac = hmac.New(sha1.New, macKey)
	} else {
		mac = hmac.New(sha256.New, macKey)
	}
	mac.Write(buf.Bytes())
	return mac.Sum(nil)
}

func writeSSHString(buf *bytes.Buffer, b []byte) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	buf.Write(n[:])
	buf.Write(b)
}

func readSSHBytes(b []byte) ([]byte, []byte, error) {
	if len(b) < 4 {
		return nil, nil, fmt.Errorf("invalid private key")
	}
	n := int(binary.BigEndian.Uint32(b[:4]))
	if n < 0 || n > len(b)-4 || n > 1<<20 {
		return nil, nil, fmt.Errorf("invalid private key")
	}
	return b[4 : 4+n], b[4+n:], nil
}

func readMPInt(b []byte) (*big.Int, []byte, error) {
	raw, rest, err := readSSHBytes(b)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > 0 && raw[0]&0x80 != 0 {
		return nil, nil, fmt.Errorf("invalid private key")
	}
	return new(big.Int).SetBytes(raw), rest, nil
}

func privateFromPPK(alg string, pub, priv []byte, encrypted bool) (any, error) {
	switch alg {
	case "ssh-ed25519":
		return ed25519FromPPK(pub, priv, encrypted)
	case "ssh-rsa":
		return rsaFromPPK(pub, priv, encrypted)
	case "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		return ecdsaFromPPK(alg, pub, priv, encrypted)
	default:
		return nil, fmt.Errorf("could not read the private key")
	}
}

func paddingOK(blob, rest []byte, encrypted bool) error {
	if !encrypted {
		if len(rest) != 0 {
			return fmt.Errorf("could not read the private key")
		}
		return nil
	}
	if len(blob)%aes.BlockSize != 0 {
		return fmt.Errorf("could not read the private key")
	}
	if len(rest) >= aes.BlockSize {
		return fmt.Errorf("could not read the private key")
	}
	return nil
}

func ed25519FromPPK(pub, priv []byte, encrypted bool) (ed25519.PrivateKey, error) {
	name, rest, err := readSSHBytes(pub)
	if err != nil || string(name) != "ssh-ed25519" {
		return nil, fmt.Errorf("could not read the private key")
	}
	rawPub, rest, err := readSSHBytes(rest)
	if err != nil || len(rest) != 0 || len(rawPub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("could not read the private key")
	}
	seed, rest, err := readSSHBytes(priv)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("could not read the private key")
	}
	if err := paddingOK(priv, rest, encrypted); err != nil {
		return nil, err
	}
	key := ed25519.NewKeyFromSeed(seed)
	if !bytes.Equal(key[ed25519.SeedSize:], rawPub) {
		return nil, fmt.Errorf("could not read the private key")
	}
	return key, nil
}

func rsaFromPPK(pub, priv []byte, encrypted bool) (*rsa.PrivateKey, error) {
	name, rest, err := readSSHBytes(pub)
	if err != nil || string(name) != "ssh-rsa" {
		return nil, fmt.Errorf("could not read the private key")
	}
	e, rest, err := readMPInt(rest)
	if err != nil {
		return nil, fmt.Errorf("could not read the private key")
	}
	n, rest, err := readMPInt(rest)
	if err != nil || len(rest) != 0 || !e.IsInt64() || e.Int64() <= 1 || e.Int64() > 1<<31-1 {
		return nil, fmt.Errorf("could not read the private key")
	}
	d, rest, err := readMPInt(priv)
	if err != nil {
		return nil, fmt.Errorf("could not read the private key")
	}
	p, rest, err := readMPInt(rest)
	if err != nil {
		return nil, fmt.Errorf("could not read the private key")
	}
	q, rest, err := readMPInt(rest)
	if err != nil {
		return nil, fmt.Errorf("could not read the private key")
	}
	qinv, rest, err := readMPInt(rest)
	if err != nil {
		return nil, fmt.Errorf("could not read the private key")
	}
	if err := paddingOK(priv, rest, encrypted); err != nil {
		return nil, err
	}
	key := &rsa.PrivateKey{
		PublicKey: rsa.PublicKey{N: n, E: int(e.Int64())},
		D:         d,
		Primes:    []*big.Int{p, q},
	}
	if err := key.Validate(); err != nil {
		return nil, fmt.Errorf("could not read the private key")
	}
	key.Precompute()
	if key.Precomputed.Qinv.Cmp(qinv) != 0 {
		return nil, fmt.Errorf("could not read the private key")
	}
	return key, nil
}

func ecdsaFromPPK(alg string, pub, priv []byte, encrypted bool) (*ecdsa.PrivateKey, error) {
	name, rest, err := readSSHBytes(pub)
	if err != nil || string(name) != alg {
		return nil, fmt.Errorf("could not read the private key")
	}
	curveName, rest, err := readSSHBytes(rest)
	if err != nil {
		return nil, fmt.Errorf("could not read the private key")
	}
	point, rest, err := readSSHBytes(rest)
	if err != nil || len(rest) != 0 {
		return nil, fmt.Errorf("could not read the private key")
	}
	var curve elliptic.Curve
	switch string(curveName) {
	case "nistp256":
		curve = elliptic.P256()
	case "nistp384":
		curve = elliptic.P384()
	case "nistp521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("could not read the private key")
	}
	if !strings.HasSuffix(alg, string(curveName)) {
		return nil, fmt.Errorf("could not read the private key")
	}
	size := (curve.Params().BitSize + 7) / 8
	if len(point) != 1+2*size || point[0] != 4 {
		return nil, fmt.Errorf("could not read the private key")
	}
	x := new(big.Int).SetBytes(point[1 : 1+size])
	y := new(big.Int).SetBytes(point[1+size:])
	d, rest, err := readMPInt(priv)
	if err != nil {
		return nil, fmt.Errorf("could not read the private key")
	}
	if err := paddingOK(priv, rest, encrypted); err != nil {
		return nil, err
	}
	if d.Sign() <= 0 || d.Cmp(curve.Params().N) >= 0 {
		return nil, fmt.Errorf("could not read the private key")
	}
	key := &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y}, D: d}
	gotX, gotY := curve.ScalarBaseMult(d.Bytes())
	if gotX.Cmp(x) != 0 || gotY.Cmp(y) != 0 {
		return nil, fmt.Errorf("could not read the private key")
	}
	return key, nil
}
