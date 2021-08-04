package gone

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/tjfoc/gmsm/sm2"
	x509sm "github.com/tjfoc/gmsm/x509"
)

// 最重要的一句话，公钥加密私钥解密，私钥签名公钥验证。

type RSABit int

const (
	RSA1024 RSABit = 1024
	RSA2048 RSABit = 2048
)

// GenerateBase64Key 生成base64编码的公私钥.
func GenerateBase64Key(secretLength SecretKeyLengthType,
	secretFormat SecretKeyFormatType) (pkStr, pbkStr string, err error) {
	var (
		privateKeyBytes []byte
		publicKeyBytes  []byte
		privateKey      *sm2.PrivateKey
		pkBytes         []byte
	)

	if secretLength == M2 {
		privateKey, err = sm2.GenerateKey(rand.Reader)
		if err != nil {
			return "", "", err
		}
		privateKeyBytes, err = x509sm.MarshalSm2UnecryptedPrivateKey(privateKey)
		if err != nil {
			return "", "", err
		}
		publicKeyBytes, err = x509sm.MarshalSm2PublicKey(&privateKey.PublicKey)
		if err != nil {
			return "", "", err
		}
		return base64.RawURLEncoding.EncodeToString(privateKeyBytes),
			base64.RawURLEncoding.EncodeToString(publicKeyBytes), nil
	}
	var priKey *rsa.PrivateKey
	if secretLength == RSA {
		priKey, err = rsa.GenerateKey(rand.Reader, 2048)
	}

	if secretFormat == PKCS1 {
		// 生成公匙
		pkStr = base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(priKey))
		pbkStr = base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&(priKey.PublicKey)))
		return pkStr, pbkStr, err
	}

	if secretFormat == PKCS8 {
		// 生成公匙
		pkBytes, err = x509.MarshalPKCS8PrivateKey(priKey)
		if err != nil {
			return "", "", err
		}
		pkStr = base64.RawURLEncoding.EncodeToString(pkBytes)
		pbkStr = base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&(priKey.PublicKey)))
		return pkStr, pbkStr, err
	}
	return "", "", err
}

// GenerateRSAKeyToMemory 生成PEM格式RSA公私钥，返回字节格式.
func GenerateRSAKeyToMemory(bits RSABit) (privateBytes []byte, publicBytes []byte, err error) {
	privateBuffer := bytes.Buffer{}
	publicBuffer := bytes.Buffer{}
	err = GenerateRSAKey(&privateBuffer, &publicBuffer, bits)
	if err != nil {
		return privateBytes, publicBytes, err
	}
	privateBytes = privateBuffer.Bytes()
	publicBytes = publicBuffer.Bytes()
	return privateBytes, publicBytes, err
}

// GenerateRSAKey 生成PEM格式RSA公私钥，写入到io.Writer中.
func GenerateRSAKey(privateWriter, publicWriter io.Writer, bits RSABit) error {
	var priKey *rsa.PrivateKey
	var pkBytes []byte
	var pbkBytes []byte
	var err error
	priKey, err = rsa.GenerateKey(rand.Reader, int(bits))
	if err != nil {
		return err
	}
	pkBytes, err = x509.MarshalPKCS8PrivateKey(priKey)
	if err != nil {
		return err
	}
	pbkBytes = x509.MarshalPKCS1PublicKey(&(priKey.PublicKey))
	err = pem.Encode(privateWriter, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: pkBytes,
	})
	if err != nil {
		return err
	}
	return pem.Encode(publicWriter, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pbkBytes,
	})
}

// GenerateSSHKey 生成ssh密钥队.
func GenerateSSHKey(bits RSABit) (pkBytes []byte, pbkBytes []byte, err error) {
	var priKey *rsa.PrivateKey
	priKey, err = rsa.GenerateKey(rand.Reader, int(bits))
	if err != nil {
		return nil, nil, err
	}
	pkBytes, err = x509.MarshalPKCS8PrivateKey(priKey)
	if err != nil {
		return nil, nil, err
	}
	pkBytes = pem.EncodeToMemory(&pem.Block{
		Bytes: pkBytes,
		Type:  "RSA PRIVATE KEY",
	})
	publicKey, err := ssh.NewPublicKey(&(priKey.PublicKey))
	if err != nil {
		return nil, nil, err
	}
	pbkBytes = ssh.MarshalAuthorizedKey(publicKey)
	return pkBytes, pbkBytes, err
}

// SignBySM2Bytes 使用sm2私钥签名字符串，返回base64编码的license.
func SignBySM2Bytes(privateKey, licenseBytes []byte) (license string, err error) {
	var key *sm2.PrivateKey
	privateKey, err = base64.RawURLEncoding.DecodeString(string(privateKey))
	if err != nil {
		return "", err
	}
	key, err = x509sm.ParsePKCS8PrivateKey(privateKey, nil)
	if err != nil {
		return "", err
	}
	return SignBySM2(key, licenseBytes)
}

// SignBySM2  使用sm2私钥对象指针签名字符串，返回base64编码的license.
func SignBySM2(privateKey *sm2.PrivateKey, licenseBytes []byte) (license string, err error) {
	var (
		signBytes        []byte
		licenseBase64Str string
	)

	// 将授权信息json编码成base64字符串
	licenseBase64Str = base64.RawURLEncoding.EncodeToString(licenseBytes)

	// 用私钥对授权信息的base64字符串进行签名
	signBytes, err = privateKey.Sign(rand.Reader, []byte(licenseBase64Str), nil)
	if err != nil {
		log.Println(err.Error())
		return
	}
	// 将签名数据编码成base64字符串
	signBase64Str := base64.RawURLEncoding.EncodeToString(signBytes)
	// 拼接完整授权字符串
	license = licenseBase64Str + "." + signBase64Str
	return
}

// SignByRSABytes 使用rsa私钥签名字符串，返回base64编码的license.
func SignByRSABytes(key, licenseBytes []byte) (string, error) {
	var (
		privateKey *rsa.PrivateKey
		pri2       interface{}
		err        error
		ok         bool
	)
	key, err = base64.RawURLEncoding.DecodeString(string(key))
	if err != nil {
		return "", err
	}
	privateKey, err = x509.ParsePKCS1PrivateKey(key)
	if err == nil {
		return SignByRSA(privateKey, licenseBytes)
	}
	pri2, err = x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	if privateKey, ok = pri2.(*rsa.PrivateKey); !ok {
		return "", errors.New("x509: failed to parse private key")
	}
	return SignByRSA(privateKey, licenseBytes)
}

// SignByRSA 使用rsa私钥对象指针签名字符串，返回base64编码的license.
func SignByRSA(key *rsa.PrivateKey, licenseBytes []byte) (license string, err error) {
	var (
		signBytes        []byte
		licenseBase64Str string
	)
	// 将授权信息json编码成base64字符串
	licenseBase64Str = base64.RawURLEncoding.EncodeToString(licenseBytes)
	hash := sha256.New()
	hash.Write([]byte(licenseBase64Str)) //nolint
	signBytes, err = key.Sign(rand.Reader, hash.Sum(nil), crypto.SHA256)
	if err != nil {
		log.Println(err.Error())
		return
	}
	// 将签名数据编码成base64字符串
	signBase64Str := base64.RawURLEncoding.EncodeToString(signBytes)

	// 拼接完整授权字符串
	license = licenseBase64Str + "." + signBase64Str
	return license, nil
}

// VerifyBySM2 使用sm2公钥验证签名的license.
func VerifyBySM2(publicKeyBase64, licenseCode string) (license string, valid bool, err error) {
	var (
		publicKeyBytes, signBytes []byte
		publicKey                 *sm2.PublicKey
		licenseInfo               []byte
	)

	// 解析公钥
	publicKeyBytes, err = base64.RawURLEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return
	}
	publicKey, err = x509sm.ParseSm2PublicKey(publicKeyBytes)
	if err != nil {
		return
	}

	// 从授权码中拆解出授权信息
	arr := strings.Split(licenseCode, ".")
	if len(arr) != 2 {
		err = errors.New("valid licenseCode")
		return
	}

	// 验证签名(被签名内容，签名)
	signBytes, err = base64.RawURLEncoding.DecodeString(arr[1])
	if err != nil {
		return
	}
	valid = publicKey.Verify([]byte(arr[0]), signBytes)
	if !valid {
		return
	}

	// 解析授权信息
	licenseInfo, err = base64.RawURLEncoding.DecodeString(arr[0])
	if err != nil {
		return "", false, err
	}
	return string(licenseInfo), true, nil
}

// VerifyByRSA 使用rsa公钥验证签名的license.
func VerifyByRSA(publicKeyBase64, licenseCode string) (license string, valid bool, err error) {
	var (
		publicKeyBytes, signBytes []byte
		publicKey                 *rsa.PublicKey
		licenseInfo               []byte
	)

	// 解析公钥
	publicKeyBytes, err = base64.RawURLEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return
	}
	publicKey, err = x509.ParsePKCS1PublicKey(publicKeyBytes)
	if err != nil {
		return
	}

	// 从授权码中拆解出授权信息
	arr := strings.Split(licenseCode, ".")
	if len(arr) != 2 {
		err = errors.New("valid licenseCode")
		return
	}

	// 验证签名(被签名内容，签名)
	signBytes, err = base64.RawURLEncoding.DecodeString(arr[1])
	if err != nil {
		return
	}

	hash := sha256.New()
	hash.Write([]byte(arr[0])) //nolint

	err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash.Sum(nil), signBytes)
	if err != nil {
		log.Println(err)
		return "", false, err
	}

	// 解析授权信息
	licenseInfo, err = base64.RawURLEncoding.DecodeString(arr[0])
	if err != nil {
		return "", false, err
	}
	return string(licenseInfo), true, nil
}

// EncryptByRSABytes 使用RSA公钥加密.
func EncryptByRSABytes(publicKey, content []byte) ([]byte, error) {
	block, _ := pem.Decode(publicKey)
	if block == nil {
		return nil, errors.New("public key error")
	}
	pi, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	// 类型断言
	var pb *rsa.PublicKey
	var ok bool
	pb, ok = pi.(*rsa.PublicKey)
	if !ok || pb == nil {
		return nil, errors.New("public key assert failed")
	}
	return EncryptByRSA(pb, content)
}

// DecryptByRSABytes 使用RSA私钥解密.
func DecryptByRSABytes(privateKey []byte, ciphertext []byte) ([]byte, error) {
	var pk *rsa.PrivateKey
	var ok bool
	block, _ := pem.Decode(privateKey)
	if block == nil {
		return nil, errors.New("private key error")
	}
	pk, errv := x509.ParsePKCS1PrivateKey(block.Bytes)
	if errv != nil {
		pi, errv := x509.ParsePKCS8PrivateKey(block.Bytes)
		if errv != nil {
			return nil, errv
		}
		pk, ok = pi.(*rsa.PrivateKey)
		if !ok || pk == nil {
			return nil, errors.New("private key assert failed")
		}
	}
	return rsa.DecryptPKCS1v15(rand.Reader, pk, ciphertext)
}

// EncryptByRSA 使用RSA公钥加密.
func EncryptByRSA(publicKey *rsa.PublicKey, content []byte) ([]byte, error) {
	return rsa.EncryptPKCS1v15(rand.Reader, publicKey, content)
}

// DecryptByRSA 使用RSA私钥解密.
func DecryptByRSA(privateKey *rsa.PrivateKey, ciphertext []byte) ([]byte, error) {
	return rsa.DecryptPKCS1v15(rand.Reader, privateKey, ciphertext)
}

// RsaPublicEncrypt Rsa公钥加密，参数publicKeyStr必须是hex、base64或者是pem编码.
func RsaPublicEncrypt(publicKeyStr string, textBytes []byte) ([]byte, error) {
	var (
		err       error
		publicKey *rsa.PublicKey
	)
	publicKeyBytes, err := DecodePemHexBase64(publicKeyStr)
	if err != nil {
		return nil, err
	}
	publicKey, err = ParsePublicKey(publicKeyBytes)
	if err != nil {
		return nil, err
	}
	return rsa.EncryptPKCS1v15(rand.Reader, publicKey, textBytes)
}

// ParsePublicKey 解析公钥，derBytes可以使用DecodePemHexBase64函数获取.
func ParsePublicKey(derBytes []byte) (publicKey *rsa.PublicKey, err error) {
	var (
		pub interface{}
		ok  bool
	)
	publicKey, err = x509.ParsePKCS1PublicKey(derBytes)
	if err == nil {
		return publicKey, nil
	}
	err = nil //nolint
	//这里不在使用pem解析，入参直接是derBytes类型
	//block, _ := pem.Decode(derBytes)
	//if block == nil {
	//	return nil, errors.New("unable to decode publicKey to request")
	//}

	pub, err = x509.ParsePKIXPublicKey(derBytes)
	if err != nil {
		return nil, errors.New("解析rsa公钥失败，你可能传递的是私钥。" + err.Error())
	}
	publicKey, ok = pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("failed to assert RSA PublicKey type")
	}
	return publicKey, nil
}

// RsaPrivateDecrypt 解析rsa私钥，参数privateKeyStr必须是hex、base64或者是pem编码.
func RsaPrivateDecrypt(privateKeyStr string, cipherBytes []byte) (textBytes []byte, err error) {
	var privateKey *rsa.PrivateKey
	derBytes, err := DecodePemHexBase64(privateKeyStr)
	if err != nil {
		return nil, err
	}
	privateKey, err = ParsePrivateKey(derBytes)
	if err != nil {
		return nil, err
	}
	return rsa.DecryptPKCS1v15(rand.Reader, privateKey, cipherBytes)
}

// ParsePrivateKey 解析私钥，derBytes可以使用DecodePemHexBase64函数获取.
func ParsePrivateKey(derBytes []byte) (privateKey *rsa.PrivateKey, err error) {
	var (
		pk interface{}
		ok bool
	)

	privateKey, err = x509.ParsePKCS1PrivateKey(derBytes)
	// if parse ok return private key
	if err == nil {
		return privateKey, nil
	}
	// 这里不在使用pem解析，入参直接是derBytes类型
	//block, _ := pem.Decode(derBytes)
	//if block == nil {
	//	return nil, errors.New("unable to decode privateKey to request")
	//}
	err = nil // nolint
	pk, err = x509.ParsePKCS8PrivateKey(derBytes)
	if err != nil {
		return nil, errors.New("解析rsa私钥失败，你可能传递的是公钥。" + err.Error())
	}
	privateKey, ok = pk.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("failed to assert RSA PrivateKey type")
	}
	return privateKey, nil
}

// DecodePemHexBase64 解析pem或者hex或者base64编码成der编码.
func DecodePemHexBase64(keyStr string) ([]byte, error) {
	if strings.Contains(keyStr, "RSA PRIVATE KEY") ||
		strings.Contains(keyStr, "PUBLIC KEY") {
		block, _ := pem.Decode([]byte(keyStr))
		if block == nil {
			return nil, errors.New("unable to decode publicKey to request")
		}
		return block.Bytes, nil
	}
	derBytes, err := hex.DecodeString(keyStr)
	// if parse ok return derBytes
	if err == nil {
		return derBytes, nil
	}
	return base64.StdEncoding.DecodeString(keyStr)
}
