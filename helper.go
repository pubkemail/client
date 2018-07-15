package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"github.com/cfromknecht/hdkey/eckey"
	"github.com/njones/base58"
	"github.com/njones/bitcoin-crypto/bitelliptic"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/ecdh"
	"golang.org/x/crypto/openpgp/packet"
)

// encData holds the relative information needed for GPG to encode data
// for the email to be encoded at rest
type encData struct {
	timestamp            time.Time
	name, comment, email string
}

// ecPublicKey defines the ASN1 for a EC Public Key
type ecPublicKey struct {
	ID            asn1.ObjectIdentifier `asn1:"optional,explicit,tag:0"`
	NamedCurveOID asn1.ObjectIdentifier `asn1:"optional,explicit,tag:1"`
	PublicKey     asn1.BitString        `asn1:"optional,explicit,tag:2"`
}

// remotePublicKey returns the pubkemail public key that coorosponds to the
// public key of an address that has been submited. This is what the shared
// key is based on.
func remotePublicKey(x []byte) ([]byte, error) {
	resp, err := http.Get(fmt.Sprintf("http://shared.pubkemail.com/public/key/%x", x))
	if err != nil {
		return nil, fmt.Errorf("http get public key: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http get status: %v", http.StatusText(resp.StatusCode))
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http get read body: %v", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("http get public key empty pem block")
	}

	block, rest := pem.Decode(body)
	if len(rest) > 0 {
		return nil, fmt.Errorf("http get public key pem block: extra data")
	}

	var ecPubKey = ecPublicKey{}
	rum, err := asn1.Unmarshal(block.Bytes, &ecPubKey)
	if err != nil {
		return nil, fmt.Errorf("http get public key unmarshal: %v", err)
	}

	if len(rum) != 0 {
		return nil, fmt.Errorf("http get public key unmarshal asn1: extra data")
	}

	return ecPubKey.PublicKey.Bytes, nil
}

// sharedKey takes the bytes of a public key and private key and returns
// the shared key based on both of them.
func sharedKey(pubKey, priKey []byte) (shrKey []byte, err error) {
	var puX, puY *big.Int
	switch pubKey[0] {
	case 0x02, 0x03:
		puX, puY, err = eccpUnmarshal(bitelliptic.S256(), pubKey)
		if err != nil {
			log.Warnf("invalid unmarshal: %v", err)
			return nil, fmt.Errorf("the public key is invalid")
		}
	case 0x04:
		puX, puY = elliptic.Unmarshal(bitelliptic.S256(), pubKey)
	default:
		log.Warnf("invalid public key type: 0x%x", pubKey[0])
		return nil, fmt.Errorf("the public key is invalid")
	}

	if !bitelliptic.S256().IsOnCurve(puX, puY) {
		log.Warnf("the public key is invalid. Not on curve")
		return nil, fmt.Errorf("the public key is invalid")
	}

	shX, shY := bitelliptic.S256().ScalarMult(puX, puY, priKey)
	return elliptic.Marshal(bitelliptic.S256(), shX, shY), nil
}

// eccpUnmarshal returns the compressed key public key pair
func eccpUnmarshal(curve elliptic.Curve, compBytes []byte) (x, y *big.Int, err error) {
	c, err := eckey.NewCompressedPublicKey(compBytes)
	if err != nil {
		return x, y, err
	}

	o, err := c.Uncompress()
	if err != nil {
		return x, y, err
	}

	x, y = o.Coords()
	return x, y, err
}

// unmarshalWIF takes a string and checks to see if it's
// a valid WIF, if so it will grab the shared key and
// return a struct that contains all of the WIF details
func unmarshalWIF(wifStr string) (w WIF, err error) {
	var wif *btcutil.WIF

	b, err := base58.BitcoinEncoding.DecodeString(wifStr)
	if err != nil {
		return w, err
	}

	if len(b) < 4 {
		return w, fmt.Errorf("wif is invalid")
	}

	wif, err = btcutil.DecodeWIF(wifStr)
	if err != nil {
		return w, err
	}

	addr, err := btcutil.NewAddressPubKey(wif.SerializePubKey(), &chaincfg.TestNet3Params)
	if err != nil {
		return w, err
	}

	w.wif = wifStr
	w.addr = addr.EncodeAddress()
	w.priKey = wif.PrivKey.D.Bytes()
	w.pubKey = addr.ScriptAddress()

	remPubKey, err := remotePublicKey(w.pubKey[len(w.pubKey)-1:])
	if err != nil {
		return w, fmt.Errorf("remote pubk: %v", err)
	}

	w.sharedKey, err = sharedKey(remPubKey, w.priKey)
	if err != nil {
		return w, err
	}

	switch b[0] {
	case 0x80:
		w.currency = "BTC"
	case 0xB0:
		w.currency = "LTC"
	case 0x9E:
		w.currency = "XDG"
	case 0xEF, 0xF1:
		w.currency = "-T-"
	}

	return w, nil
}

// randPrefix returns a base58 encoded random bytes. Base58
// was choosen becuase it's URL friendly and easy to type
// for users who may not copy and paste the random part of
// the URL.
func randPrefix(l int) string {
	wrp := make([]byte, l)
	rand.Read(wrp)
	return base58.BitcoinEncoding.EncodeToString(wrp)
}

// txtTruncate takes a string of any length and truncates it
// if necessary and uses ellipis to indicate more
func txtTruncate(text string, length int) string {
	if len(text) > 3 && len(text) > length {
		return text[:len(text)-3] + "..."
	}
	return text
}

// addrDataMapToString takes a map of addresses and related data
// and returns a sorted string of each address on a row with
// the related data in a standard format
func addrDataMapToString(m map[string]addrData) string {
	lines, keys := make([]string, 0, len(m)), make([]string, 0, len(m))

	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m[k]
		lines = append(lines, fmt.Sprintf(" %s %s", v.wif.currency, txtTruncate(k, 32)))
	}

	return strings.Join(lines, "\n")
}

// newEntity combines and calculates the entity data for PGP encryption
func newEntity(data encData, sigKey *ecdsa.PrivateKey, encKey *ecdh.PrivateKey) (*openpgp.Entity, error) {
	uid := packet.NewUserId(data.name, data.comment, data.email)
	if uid == nil {
		return nil, fmt.Errorf("bad user id, invalid character found")
	}

	entity := &openpgp.Entity{
		PrimaryKey: packet.NewECDSAPublicKey(data.timestamp, &sigKey.PublicKey),
		PrivateKey: packet.NewECDSAPrivateKey(data.timestamp, sigKey),
		Identities: make(map[string]*openpgp.Identity),
	}

	isPrimaryID := false

	entity.Identities[uid.Id] = &openpgp.Identity{
		Name:   uid.Name,
		UserId: uid,
		SelfSignature: &packet.Signature{
			CreationTime: time.Now(),
			SigType:      packet.SigTypePositiveCert,
			PubKeyAlgo:   packet.PubKeyAlgoECDSA,
			Hash:         crypto.RIPEMD160,
			IsPrimaryId:  &isPrimaryID,
			FlagsValid:   true,
			FlagSign:     true,
			FlagCertify:  true,
			IssuerKeyId:  &entity.PrimaryKey.KeyId,
		},
	}

	keyLifetimeSecs := uint32(86400 * 365)
	encprik := packet.NewECDHPrivateKey(data.timestamp, encKey)

	entity.Subkeys = make([]openpgp.Subkey, 1)
	entity.Subkeys[0] = openpgp.Subkey{
		PublicKey:  &encprik.PublicKey,
		PrivateKey: encprik,
		Sig: &packet.Signature{
			CreationTime:              time.Now(),
			SigType:                   packet.SigTypeSubkeyBinding,
			PubKeyAlgo:                packet.PubKeyAlgoECDH,
			Hash:                      crypto.RIPEMD160,
			FlagsValid:                true,
			FlagEncryptStorage:        true,
			FlagEncryptCommunications: true,
			IssuerKeyId:               &entity.PrimaryKey.KeyId,
			KeyLifetimeSecs:           &keyLifetimeSecs,
		},
	}

	entity.Subkeys[0].PublicKey.IsSubkey = true
	entity.Subkeys[0].PrivateKey.IsSubkey = true
	return entity, nil
}
