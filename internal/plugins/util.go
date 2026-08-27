package plugins

import (
	"bytes"
	"crypto/rand"
	"time"

	"gopkg.in/yaml.v3"
)

var cryptoRandRead = rand.Read

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func yamlUnmarshalStrict(raw []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	return decoder.Decode(target)
}
