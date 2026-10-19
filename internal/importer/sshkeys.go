package importer

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
)

// validateSSHPrivateKey deliberately accepts only formats we can validate
// offline. OpenSSH bcrypt-encrypted keys and encrypted PKCS8 require additional
// crypto dependencies and are rejected rather than handed unchecked to a core.
func validateSSHPrivateKey(value, passphrase string) error {
	if !inlinePrivateKey(value) {
		return errors.New("SSH private keys must be inline PEM; file paths are forbidden")
	}
	block, _ := pem.Decode([]byte(value))
	data := block.Bytes
	if x509.IsEncryptedPEMBlock(block) {
		if passphrase == "" {
			return errors.New("encrypted SSH private key requires a passphrase")
		}
		var err error
		data, err = x509.DecryptPEMBlock(block, []byte(passphrase))
		if err != nil {
			return errors.New("invalid encrypted SSH private key or passphrase")
		}
	} else if passphrase != "" && block.Type != "OPENSSH PRIVATE KEY" {
		return errors.New("passphrase supplied for an unencrypted SSH private key")
	}
	var key any
	var err error
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(data)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(data)
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(data)
	case "OPENSSH PRIVATE KEY":
		return validateOpenSSHPrivate(data, passphrase)
	default:
		return errors.New("unsupported inline SSH private key format")
	}
	if err != nil {
		return errors.New("invalid inline SSH private key")
	}
	switch k := key.(type) {
	case *rsa.PrivateKey:
		if k.Validate() != nil {
			return errors.New("invalid RSA private key")
		}
	case *ecdsa.PrivateKey:
		if k.D.Sign() <= 0 || !k.Curve.IsOnCurve(k.X, k.Y) {
			return errors.New("invalid ECDSA private key")
		}
	case ed25519.PrivateKey:
		if len(k) != ed25519.PrivateKeySize {
			return errors.New("invalid Ed25519 private key")
		}
	default:
		return errors.New("unsupported SSH private key algorithm")
	}
	return nil
}

type sshWire struct {
	data []byte
	bad  bool
}

func (w *sshWire) number() uint32 {
	if len(w.data) < 4 {
		w.bad = true
		return 0
	}
	n := binary.BigEndian.Uint32(w.data[:4])
	w.data = w.data[4:]
	return n
}
func (w *sshWire) field() []byte {
	n := w.number()
	if w.bad || uint64(n) > uint64(len(w.data)) {
		w.bad = true
		return nil
	}
	value := w.data[:int(n)]
	w.data = w.data[int(n):]
	return value
}
func sshCurve(name string) elliptic.Curve {
	switch name {
	case "nistp256":
		return elliptic.P256()
	case "nistp384":
		return elliptic.P384()
	case "nistp521":
		return elliptic.P521()
	}
	return nil
}
func validateSSHPublicBlob(blob []byte, algorithm string) bool {
	w := sshWire{data: blob}
	if string(w.field()) != algorithm {
		return false
	}
	switch algorithm {
	case "ssh-ed25519":
		if len(w.field()) != ed25519.PublicKeySize {
			return false
		}
	case "ssh-rsa":
		exponent := w.field()
		modulus := w.field()
		if len(exponent) == 0 || len(exponent) > 4 || len(modulus) < 64 || len(modulus) > 2049 {
			return false
		}
		e := new(big.Int).SetBytes(exponent)
		n := new(big.Int).SetBytes(modulus)
		if e.Sign() <= 0 || e.Bit(0) == 0 || n.Sign() <= 0 || n.Bit(0) == 0 {
			return false
		}
	case "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		curveName := string(w.field())
		curve := sshCurve(curveName)
		if curve == nil || algorithm != "ecdsa-sha2-"+curveName {
			return false
		}
		x, y := elliptic.Unmarshal(curve, w.field())
		if x == nil || y == nil {
			return false
		}
	default:
		return false
	}
	return !w.bad && len(w.data) == 0
}
func normalizeSSHHostKey(value string) (string, error) {
	parts := strings.Fields(value)
	if len(parts) < 2 {
		return "", errors.New("invalid SSH host public key")
	}
	blob, err := base64.StdEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		blob, err = base64.RawStdEncoding.Strict().DecodeString(parts[1])
	}
	if err != nil || !validateSSHPublicBlob(blob, parts[0]) {
		return "", errors.New("unsupported or invalid SSH host public key")
	}
	return parts[0] + " " + base64.StdEncoding.EncodeToString(blob), nil
}
func validateOpenSSHPrivate(data []byte, passphrase string) error {
	const magic = "openssh-key-v1\x00"
	invalid := errors.New("invalid or unsupported inline OpenSSH private key")
	if !bytes.HasPrefix(data, []byte(magic)) {
		return invalid
	}
	w := sshWire{data: data[len(magic):]}
	cipher, kdf, options := string(w.field()), string(w.field()), w.field()
	if cipher != "none" || kdf != "none" || len(options) != 0 {
		return errors.New("encrypted OpenSSH private keys are unsupported; use unencrypted inline PEM or password authentication")
	}
	if passphrase != "" {
		return errors.New("passphrase supplied for an unencrypted SSH private key")
	}
	if w.number() != 1 {
		return invalid
	}
	public, private := w.field(), w.field()
	if w.bad || len(w.data) != 0 || len(private)%8 != 0 {
		return invalid
	}
	p := sshWire{data: private}
	if p.number() != p.number() {
		return invalid
	}
	algorithm := string(p.field())
	if !validateSSHPublicBlob(public, algorithm) {
		return invalid
	}
	pub := sshWire{data: public}
	pub.field()
	switch algorithm {
	case "ssh-ed25519":
		pubBytes, privBytes := p.field(), p.field()
		if len(pubBytes) != ed25519.PublicKeySize || len(privBytes) != ed25519.PrivateKeySize || !bytes.Equal(pubBytes, pub.field()) {
			return invalid
		}
		generated := ed25519.NewKeyFromSeed(privBytes[:ed25519.SeedSize])
		if !bytes.Equal(privBytes, generated) || !bytes.Equal(pubBytes, generated[ed25519.SeedSize:]) {
			return invalid
		}
	case "ssh-rsa":
		n, e, d, iqmp, prime1, prime2 := p.field(), p.field(), p.field(), p.field(), p.field(), p.field()
		if !bytes.Equal(e, pub.field()) || !bytes.Equal(n, pub.field()) || len(e) > 4 {
			return invalid
		}
		privateKey := &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, D: new(big.Int).SetBytes(d), Primes: []*big.Int{new(big.Int).SetBytes(prime1), new(big.Int).SetBytes(prime2)}}
		if privateKey.Validate() != nil {
			return invalid
		}
		inverse := new(big.Int).ModInverse(privateKey.Primes[1], privateKey.Primes[0])
		if inverse == nil || inverse.Cmp(new(big.Int).SetBytes(iqmp)) != 0 {
			return invalid
		}
	case "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		curveName, point, d := p.field(), p.field(), p.field()
		if !bytes.Equal(curveName, pub.field()) || !bytes.Equal(point, pub.field()) {
			return invalid
		}
		curve := sshCurve(string(curveName))
		if curve == nil {
			return invalid
		}
		scalar := new(big.Int).SetBytes(d)
		if scalar.Sign() <= 0 || scalar.Cmp(curve.Params().N) >= 0 {
			return invalid
		}
		x, y := curve.ScalarBaseMult(d)
		if !bytes.Equal(elliptic.Marshal(curve, x, y), point) {
			return invalid
		}
	default:
		return invalid
	}
	p.field() // key comment is not configuration and is never echoed
	if p.bad || len(p.data) == 0 {
		return invalid
	}
	for i, b := range p.data {
		if int(b) != (i+1)%256 {
			return invalid
		}
	}
	return nil
}
