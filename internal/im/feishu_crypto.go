package im

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
)

// FeishuEncryptData 飞书加密数据结构
type FeishuEncryptData struct {
	Encrypt string `json:"encrypt"` // base64 编码的加密数据
}

// DecryptFeishuEvent 解密飞书事件
// key: 飞书 App Secret（用作 AES 密钥）
// encryptData: 加密的数据 {"encrypt":"..."}
func DecryptFeishuEvent(encryptKey string, encryptData string) ([]byte, error) {
	// 1. 解析加密数据
	var enc FeishuEncryptData
	if err := json.Unmarshal([]byte(encryptData), &enc); err != nil {
		return nil, fmt.Errorf("解析加密数据失败: %w", err)
	}

	// 2. Base64 解码密文
	cipherText, err := base64.StdEncoding.DecodeString(enc.Encrypt)
	if err != nil {
		return nil, fmt.Errorf("base64 解码失败: %w", err)
	}

	// 3. 生成 AES 密钥（16 字节）
	// App Secret 可能超过 16 字节，取前 16 字节或使用派生密钥
	key := make([]byte, 16)
	if len(encryptKey) >= 16 {
		key = []byte(encryptKey)[:16]
	} else {
		// 如果密钥不足 16 字节，用 0 填充
		copy(key, []byte(encryptKey))
	}

	// 4. AES-128-CBC 解密
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES 密钥错误: %w", err)
	}

	// 5. 提取 IV 和实际密文
	// 飞书加密格式：IV(16字节) + 密文
	if len(cipherText) < aes.BlockSize {
		return nil, fmt.Errorf("密文长度不足")
	}

	iv := cipherText[:aes.BlockSize]
	encrypted := cipherText[aes.BlockSize:]

	// 6. 检查密文长度是否为 16 的倍数
	if len(encrypted)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("密文长度必须是 16 的倍数")
	}

	// 7. CBC 解密
	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(encrypted))
	mode.CryptBlocks(plaintext, encrypted)

	// 8. 移除 PKCS#7 填充
	plaintext = removePKCS7Padding(plaintext)

	return plaintext, nil
}

// removePKCS7Padding 移除 PKCS#7 填充
func removePKCS7Padding(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	padding := int(data[len(data)-1])
	if padding < 1 || padding > aes.BlockSize {
		return data
	}
	return data[:len(data)-padding]
}

// DebugFunc 调试函数
func DebugFeishuDecrypt(encryptKey, encryptData string) {
	plaintext, err := DecryptFeishuEvent(encryptKey, encryptData)
	if err != nil {
		log.Printf("[Feishu] 解密失败: %v\n", err)
		return
	}
	log.Printf("[Feishu] 解密成功: %s\n", string(plaintext))
}
