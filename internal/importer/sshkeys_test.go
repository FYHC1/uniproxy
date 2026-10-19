package importer

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"net/url"
	"strings"
	"testing"
)

func testSSHPrivatePEM(t *testing.T) string {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}
func wireNumber(n uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, n); return b }
func wireFields(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, wireNumber(uint32(len(p)))...)
		out = append(out, p...)
	}
	return out
}
func makeOpenSSH(algorithm string, publicFields, privateFields []byte) []byte {
	public := append(wireFields([]byte(algorithm)), publicFields...)
	private := append(wireNumber(42), wireNumber(42)...)
	private = append(private, wireFields([]byte(algorithm))...)
	private = append(private, privateFields...)
	private = append(private, wireFields([]byte("fixture"))...)
	for i := 1; ; i++ {
		private = append(private, byte(i))
		if len(private)%8 == 0 {
			break
		}
	}
	data := append([]byte("openssh-key-v1\x00"), wireFields([]byte("none"), []byte("none"), nil)...)
	data = append(data, wireNumber(1)...)
	return append(data, wireFields(public, private)...)
}
func pemKey(kind string, data []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: data}))
}
func TestSSHPrivateKeyFormats(t *testing.T) {
	edKey := ed25519.NewKeyFromSeed(make([]byte, 32))
	edPub := edKey[32:]
	ecdsaKey := &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: elliptic.P256()}, D: big.NewInt(1)}
	ecdsaKey.X, ecdsaKey.Y = ecdsaKey.Curve.ScalarBaseMult(ecdsaKey.D.Bytes())
	ecdsaDER, err := x509.MarshalECPrivateKey(ecdsaKey)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey.Precompute()
	cases := map[string]string{
		"pkcs8-ed25519":   testSSHPrivatePEM(t),
		"pkcs1-rsa":       pemKey("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(rsaKey)),
		"sec1-ecdsa":      pemKey("EC PRIVATE KEY", ecdsaDER),
		"openssh-ed25519": pemKey("OPENSSH PRIVATE KEY", makeOpenSSH("ssh-ed25519", wireFields(edPub), wireFields(edPub, edKey))),
		"openssh-ecdsa":   pemKey("OPENSSH PRIVATE KEY", makeOpenSSH("ecdsa-sha2-nistp256", wireFields([]byte("nistp256"), elliptic.Marshal(ecdsaKey.Curve, ecdsaKey.X, ecdsaKey.Y)), wireFields([]byte("nistp256"), elliptic.Marshal(ecdsaKey.Curve, ecdsaKey.X, ecdsaKey.Y), ecdsaKey.D.Bytes()))),
		"openssh-rsa":     pemKey("OPENSSH PRIVATE KEY", makeOpenSSH("ssh-rsa", wireFields(big.NewInt(int64(rsaKey.E)).Bytes(), rsaKey.N.Bytes()), wireFields(rsaKey.N.Bytes(), big.NewInt(int64(rsaKey.E)).Bytes(), rsaKey.D.Bytes(), rsaKey.Precomputed.Qinv.Bytes(), rsaKey.Primes[0].Bytes(), rsaKey.Primes[1].Bytes()))),
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			obj := nativeFixture("ssh")
			delete(obj, "password")
			obj["private-key"] = key
			n := parseOne(t, nativeDoc(t, obj))
			if n.Config["private-key"] != key {
				t.Fatal("key content changed")
			}
			uri := "ssh://root@example.com?private-key=" + url.QueryEscape(key)
			parseOne(t, uri)
		})
	}
	encrypted, err := x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", ecdsaDER, []byte("secret"), x509.PEMCipherAES256)
	if err != nil {
		t.Fatal(err)
	}
	obj := nativeFixture("ssh")
	obj["private-key"] = string(pem.EncodeToMemory(encrypted))
	obj["private-key-passphrase"] = "secret"
	parseOne(t, nativeDoc(t, obj))
	obj["private-key-passphrase"] = "wrong"
	requireInvalid(t, nativeDoc(t, obj))
	delete(obj, "private-key-passphrase")
	requireInvalid(t, nativeDoc(t, obj))
}
func TestSSHRejectMalformedKeys(t *testing.T) {
	edKey := ed25519.NewKeyFromSeed(make([]byte, 32))
	data := makeOpenSSH("ssh-ed25519", wireFields(edKey[32:]), wireFields(edKey[32:], edKey))
	for _, key := range []string{
		"C:\\private\\id_rsa", pemKey("OPENSSH PRIVATE KEY", []byte("garbage")), pemKey("PRIVATE KEY", []byte("garbage")), pemKey("RSA PRIVATE KEY", []byte("garbage")), pemKey("EC PRIVATE KEY", []byte("garbage")), pemKey("DSA PRIVATE KEY", []byte("garbage")), pemKey("ENCRYPTED PRIVATE KEY", []byte("garbage")), testSSHPrivatePEM(t) + "trailing-data",
	} {
		obj := nativeFixture("ssh")
		obj["private-key"] = key
		requireInvalid(t, nativeDoc(t, obj))
	}
	for i := 0; i < len(data); i++ {
		copy := append([]byte{}, data[:i]...)
		if validateSSHPrivateKey(pemKey("OPENSSH PRIVATE KEY", copy), "") == nil {
			t.Fatalf("truncated OpenSSH key accepted at %d", i)
		}
	}
	for _, index := range []int{0, len(data) - 1, len(data) - 2} {
		bad := append([]byte{}, data...)
		bad[index] ^= 0xff
		if validateSSHPrivateKey(pemKey("OPENSSH PRIVATE KEY", bad), "") == nil {
			t.Fatal("corrupt OpenSSH key accepted")
		}
	}
	encrypted := append([]byte("openssh-key-v1\x00"), wireFields([]byte("aes256-ctr"), []byte("bcrypt"), []byte("options"))...)
	if validateSSHPrivateKey(pemKey("OPENSSH PRIVATE KEY", encrypted), "password") == nil {
		t.Fatal("unvalidated encrypted OpenSSH accepted")
	}
	obj := nativeFixture("ssh")
	obj["private-key"] = testSSHPrivatePEM(t)
	obj["private-key-passphrase"] = "unused"
	requireInvalid(t, nativeDoc(t, obj))
}
func TestSSHHostPinValidation(t *testing.T) {
	edKey := ed25519.NewKeyFromSeed(make([]byte, 32))
	blob := wireFields([]byte("ssh-ed25519"), edKey[32:])
	key := "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob)
	obj := nativeFixture("ssh")
	obj["host-key"] = []string{key + " comment"}
	n := parseOne(t, nativeDoc(t, obj))
	if n.Config["host-key"].([]string)[0] != key {
		t.Fatal("host key comment not canonicalized")
	}
	for _, bad := range []string{"/etc/host-key", "ssh-ed25519 invalid", "ssh-rsa " + base64.StdEncoding.EncodeToString(blob), "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob[:len(blob)-1]), "ssh-ed25519 " + base64.StdEncoding.EncodeToString(append(blob, 1))} {
		obj["host-key"] = []string{bad}
		requireInvalid(t, nativeDoc(t, obj))
	}
	for _, bad := range []string{"rsa", "ssh-unknown", "ssh-dss", "ssh-ed25519-cert-v01@openssh.com"} {
		obj = nativeFixture("ssh")
		obj["host-key-algorithms"] = []string{bad}
		requireInvalid(t, nativeDoc(t, obj))
	}
	if _, err := normalizeSSHHostKey(strings.Repeat(" ", 10)); err == nil {
		t.Fatal("empty key accepted")
	}
}
func FuzzSSHKeysNeverPanic(f *testing.F) {
	edKey := ed25519.NewKeyFromSeed(make([]byte, 32))
	data := makeOpenSSH("ssh-ed25519", wireFields(edKey[32:]), wireFields(edKey[32:], edKey))
	f.Add(data)
	f.Add([]byte("openssh-key-v1\x00"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 65536 {
			return
		}
		validateOpenSSHPrivate(b, "")
		validateSSHPublicBlob(b, "ssh-ed25519")
		validateSSHPublicBlob(b, "ssh-rsa")
		validateSSHPublicBlob(b, "ecdsa-sha2-nistp256")
	})
}
