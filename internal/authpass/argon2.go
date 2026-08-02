package authpass

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16

	maxArgonTime    = 6
	maxArgonMemory  = 256 * 1024
	maxArgonThreads = 8
)

func Hash(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func Verify(encoded, password string) bool {
	want, err := decode(encoded)
	if err != nil {
		dummyVerify(password)
		return false
	}
	got := argon2.IDKey([]byte(password), want.salt, want.time, want.memory, want.threads, uint32(len(want.hash)))
	return subtle.ConstantTimeCompare(got, want.hash) == 1
}

func Validate(encoded string) error {
	_, err := decode(encoded)
	return err
}

func dummyVerify(password string) {
	salt := make([]byte, argonSaltLen)
	_ = argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

type decoded struct {
	salt    []byte
	hash    []byte
	time    uint32
	memory  uint32
	threads uint8
}

func decode(encoded string) (decoded, error) {
	encoded = strings.TrimSpace(encoded)
	encoded = strings.TrimPrefix(encoded, "$")
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" {
		return decoded{}, errors.New("invalid hash")
	}
	if !strings.HasPrefix(parts[1], "v=") {
		return decoded{}, errors.New("invalid hash version")
	}
	var mem, timeCost, threads uint64
	for _, p := range strings.Split(parts[2], ",") {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			return decoded{}, errors.New("invalid hash params")
		}
		n, err := strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			return decoded{}, err
		}
		switch kv[0] {
		case "m":
			mem = n
		case "t":
			timeCost = n
		case "p":
			threads = n
		}
	}
	if mem == 0 || timeCost == 0 || threads == 0 {
		return decoded{}, errors.New("invalid hash params")
	}
	if mem > maxArgonMemory || timeCost > maxArgonTime || threads > maxArgonThreads {
		return decoded{}, errors.New("hash params too large")
	}
	if threads > 255 {
		return decoded{}, errors.New("invalid hash params")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) < 8 {
		return decoded{}, errors.New("invalid hash salt")
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(hash) < 16 {
		return decoded{}, errors.New("invalid hash digest")
	}
	return decoded{
		salt:    salt,
		hash:    hash,
		time:    uint32(timeCost),
		memory:  uint32(mem),
		threads: uint8(threads),
	}, nil
}
