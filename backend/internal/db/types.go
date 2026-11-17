package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"fulfillment/internal/crypt"
)

// JSONField stores any JSON-serialisable value in a text column. It works on
// both PostgreSQL and SQLite.
type JSONField[T any] struct{ V T }

func J[T any](v T) JSONField[T] { return JSONField[T]{V: v} }

func (JSONField[T]) GormDataType() string { return "text" }

func (j JSONField[T]) Value() (driver.Value, error) {
	b, err := json.Marshal(j.V)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (j *JSONField[T]) Scan(src any) error {
	var b []byte
	switch s := src.(type) {
	case nil:
		var zero T
		j.V = zero
		return nil
	case string:
		b = []byte(s)
	case []byte:
		b = s
	default:
		return fmt.Errorf("db: cannot scan %T into JSONField", src)
	}
	if len(b) == 0 {
		var zero T
		j.V = zero
		return nil
	}
	return json.Unmarshal(b, &j.V)
}

func (j JSONField[T]) MarshalJSON() ([]byte, error) { return json.Marshal(j.V) }

func (j *JSONField[T]) UnmarshalJSON(b []byte) error { return json.Unmarshal(b, &j.V) }

// Secret is a string encrypted at rest with the process master secret.
type Secret string

func (Secret) GormDataType() string { return "text" }

func (s Secret) Value() (driver.Value, error) {
	enc, err := crypt.Encrypt(string(s))
	if err != nil {
		return nil, err
	}
	return enc, nil
}

func (s *Secret) Scan(src any) error {
	var str string
	switch v := src.(type) {
	case nil:
		*s = ""
		return nil
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("db: cannot scan %T into Secret", src)
	}
	plain, err := crypt.Decrypt(str)
	if err != nil {
		return err
	}
	*s = Secret(plain)
	return nil
}

func (s Secret) String() string { return string(s) }

// MarshalJSON never leaks the secret.
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`""`), nil }

func Ptr[T any](v T) *T { return &v }

func Now() time.Time { return time.Now().UTC() }
