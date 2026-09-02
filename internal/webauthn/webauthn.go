package webauthn

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

var b64 = base64.RawURLEncoding

type ClientData struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Origin    string `json:"origin"`
}

type RegistrationResult struct {
	CredentialID []byte
	PublicKeyDER []byte
	SignCount    uint32
}

func DecodeBase64URL(v string) ([]byte, error) { return b64.DecodeString(v) }
func EncodeBase64URL(v []byte) string          { return b64.EncodeToString(v) }

func VerifyRegistration(clientDataJSON, attestationObject []byte, expectedChallenge, expectedOrigin, rpID string) (RegistrationResult, error) {
	if _, err := verifyClientData(clientDataJSON, "webauthn.create", expectedChallenge, expectedOrigin); err != nil {
		return RegistrationResult{}, err
	}
	root, _, err := decodeCBOR(attestationObject)
	if err != nil {
		return RegistrationResult{}, fmt.Errorf("attestation CBOR: %w", err)
	}
	authDataValue, ok := mapString(root, "authData")
	if !ok {
		return RegistrationResult{}, errors.New("attestation missing authData")
	}
	authData, ok := authDataValue.([]byte)
	if !ok || len(authData) < 55 {
		return RegistrationResult{}, errors.New("authenticator data invalid")
	}
	if err := verifyRPHash(authData, rpID); err != nil {
		return RegistrationResult{}, err
	}
	flags := authData[32]
	if flags&0x01 == 0 || flags&0x40 == 0 {
		return RegistrationResult{}, errors.New("authenticator data missing UP/AT flags")
	}
	signCount := binary.BigEndian.Uint32(authData[33:37])
	credLen := int(binary.BigEndian.Uint16(authData[53:55]))
	if credLen <= 0 || 55+credLen >= len(authData) {
		return RegistrationResult{}, errors.New("credential id length invalid")
	}
	credentialID := append([]byte(nil), authData[55:55+credLen]...)
	coseValue, _, err := decodeCBOR(authData[55+credLen:])
	if err != nil {
		return RegistrationResult{}, fmt.Errorf("credential public key CBOR: %w", err)
	}
	pub, err := cosePublicKey(coseValue)
	if err != nil {
		return RegistrationResult{}, err
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return RegistrationResult{}, err
	}
	return RegistrationResult{CredentialID: credentialID, PublicKeyDER: der, SignCount: signCount}, nil
}

func VerifyAssertion(clientDataJSON, authenticatorData, signature []byte, expectedChallenge, expectedOrigin, rpID string, publicKeyDER []byte) (uint32, error) {
	if _, err := verifyClientData(clientDataJSON, "webauthn.get", expectedChallenge, expectedOrigin); err != nil {
		return 0, err
	}
	if len(authenticatorData) < 37 {
		return 0, errors.New("authenticator data too short")
	}
	if err := verifyRPHash(authenticatorData, rpID); err != nil {
		return 0, err
	}
	if authenticatorData[32]&0x01 == 0 {
		return 0, errors.New("user presence flag missing")
	}
	pub, err := x509.ParsePKIXPublicKey(publicKeyDER)
	if err != nil {
		return 0, fmt.Errorf("stored public key: %w", err)
	}
	clientHash := sha256.Sum256(clientDataJSON)
	signed := append(append([]byte(nil), authenticatorData...), clientHash[:]...)
	digest := sha256.Sum256(signed)
	switch key := pub.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(key, digest[:], signature) {
			return 0, errors.New("passkey signature invalid")
		}
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
			return 0, errors.New("passkey signature invalid")
		}
	default:
		return 0, fmt.Errorf("unsupported public key type %T", pub)
	}
	return binary.BigEndian.Uint32(authenticatorData[33:37]), nil
}

func verifyClientData(raw []byte, wantType, wantChallenge, wantOrigin string) (ClientData, error) {
	var cd ClientData
	if err := json.Unmarshal(raw, &cd); err != nil {
		return cd, fmt.Errorf("clientDataJSON: %w", err)
	}
	if cd.Type != wantType {
		return cd, errors.New("webauthn client data type mismatch")
	}
	if cd.Challenge != wantChallenge {
		return cd, errors.New("webauthn challenge mismatch")
	}
	if cd.Origin != wantOrigin {
		return cd, errors.New("webauthn origin mismatch")
	}
	return cd, nil
}

func verifyRPHash(authData []byte, rpID string) error {
	want := sha256.Sum256([]byte(rpID))
	if len(authData) < 32 || !bytes.Equal(authData[:32], want[:]) {
		return errors.New("relying party id hash mismatch")
	}
	return nil
}

type cborPair struct{ Key, Value any }

func mapString(v any, key string) (any, bool) {
	pairs, ok := v.([]cborPair)
	if !ok {
		return nil, false
	}
	for _, p := range pairs {
		if k, ok := p.Key.(string); ok && k == key {
			return p.Value, true
		}
	}
	return nil, false
}

func mapInt(v any, key int64) (any, bool) {
	pairs, ok := v.([]cborPair)
	if !ok {
		return nil, false
	}
	for _, p := range pairs {
		if k, ok := p.Key.(int64); ok && k == key {
			return p.Value, true
		}
	}
	return nil, false
}

func cosePublicKey(v any) (any, error) {
	ktyV, ok := mapInt(v, 1)
	if !ok {
		return nil, errors.New("COSE key missing kty")
	}
	kty, ok := ktyV.(int64)
	if !ok {
		return nil, errors.New("COSE kty invalid")
	}
	switch kty {
	case 2:
		crvV, _ := mapInt(v, -1)
		xV, _ := mapInt(v, -2)
		yV, _ := mapInt(v, -3)
		crv, _ := crvV.(int64)
		x, xok := xV.([]byte)
		y, yok := yV.([]byte)
		if crv != 1 || !xok || !yok {
			return nil, errors.New("unsupported EC COSE key")
		}
		pk := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !pk.Curve.IsOnCurve(pk.X, pk.Y) {
			return nil, errors.New("EC public key is not on P-256")
		}
		return pk, nil
	case 3:
		nV, _ := mapInt(v, -1)
		eV, _ := mapInt(v, -2)
		n, nok := nV.([]byte)
		eBytes, eok := eV.([]byte)
		if !nok || !eok || len(eBytes) == 0 || len(eBytes) > 4 {
			return nil, errors.New("invalid RSA COSE key")
		}
		e := 0
		for _, b := range eBytes {
			e = (e << 8) | int(b)
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: e}, nil
	default:
		return nil, fmt.Errorf("unsupported COSE kty %d", kty)
	}
}

func decodeCBOR(data []byte) (any, int, error) {
	d := cborDecoder{b: data}
	v, err := d.item()
	return v, d.off, err
}

type cborDecoder struct {
	b   []byte
	off int
}

func (d *cborDecoder) item() (any, error) {
	if d.off >= len(d.b) {
		return nil, errors.New("unexpected end of CBOR")
	}
	first := d.b[d.off]
	d.off++
	major, ai := first>>5, first&0x1f
	n, err := d.arg(ai)
	if err != nil {
		return nil, err
	}
	switch major {
	case 0:
		if n > 1<<63-1 {
			return nil, errors.New("CBOR uint too large")
		}
		return int64(n), nil
	case 1:
		if n > 1<<63-1 {
			return nil, errors.New("CBOR negative int too large")
		}
		return -1 - int64(n), nil
	case 2:
		if err := d.need(n); err != nil {
			return nil, err
		}
		v := append([]byte(nil), d.b[d.off:d.off+int(n)]...)
		d.off += int(n)
		return v, nil
	case 3:
		if err := d.need(n); err != nil {
			return nil, err
		}
		v := string(d.b[d.off : d.off+int(n)])
		d.off += int(n)
		return v, nil
	case 4:
		arr := make([]any, 0, n)
		for i := uint64(0); i < n; i++ {
			v, err := d.item()
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	case 5:
		pairs := make([]cborPair, 0, n)
		for i := uint64(0); i < n; i++ {
			k, err := d.item()
			if err != nil {
				return nil, err
			}
			v, err := d.item()
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, cborPair{k, v})
		}
		return pairs, nil
	case 6:
		return d.item()
	case 7:
		switch ai {
		case 20:
			return false, nil
		case 21:
			return true, nil
		case 22, 23:
			return nil, nil
		}
	}
	return nil, fmt.Errorf("unsupported CBOR major=%d ai=%d", major, ai)
}

func (d *cborDecoder) arg(ai byte) (uint64, error) {
	switch {
	case ai < 24:
		return uint64(ai), nil
	case ai == 24:
		if err := d.need(1); err != nil {
			return 0, err
		}
		v := d.b[d.off]
		d.off++
		return uint64(v), nil
	case ai == 25:
		if err := d.need(2); err != nil {
			return 0, err
		}
		v := binary.BigEndian.Uint16(d.b[d.off:])
		d.off += 2
		return uint64(v), nil
	case ai == 26:
		if err := d.need(4); err != nil {
			return 0, err
		}
		v := binary.BigEndian.Uint32(d.b[d.off:])
		d.off += 4
		return uint64(v), nil
	case ai == 27:
		if err := d.need(8); err != nil {
			return 0, err
		}
		v := binary.BigEndian.Uint64(d.b[d.off:])
		d.off += 8
		return v, nil
	default:
		return 0, errors.New("indefinite/unsupported CBOR length")
	}
}

func (d *cborDecoder) need(n uint64) error {
	if n > uint64(len(d.b)-d.off) {
		return errors.New("unexpected end of CBOR")
	}
	return nil
}
