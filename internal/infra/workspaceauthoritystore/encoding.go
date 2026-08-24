package workspaceauthoritystore

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// EncodeComplete is the single executable publication-size contract for the
// final authority store. It stops before retaining more than
// MaxAuthorityBytes and does not marshal the complete aggregate into an
// unbounded intermediate buffer.
func EncodeComplete(collection tobari.WorkspaceAuthorityCollection) ([]byte, error) {
	if err := validateCollectionBounds(collection); err != nil {
		return nil, err
	}
	buffer := boundedJSONBuffer{maximum: MaxAuthorityBytes}
	if err := writeJSONValue(&buffer, reflect.ValueOf(collection)); err != nil {
		return nil, fmt.Errorf("encode final Workspace authority: %w", err)
	}
	if buffer.Len() == 0 {
		return nil, fmt.Errorf("encoded final Workspace authority is empty")
	}
	// Size is established first so semantic validation never needs to create a
	// second over-bound aggregate merely to reject an unreadable publication.
	if err := collection.Validate(); err != nil {
		return nil, fmt.Errorf("validate final Workspace authority before encoding: %w", err)
	}
	return append([]byte(nil), buffer.Bytes()...), nil
}

type boundedJSONBuffer struct {
	bytes.Buffer
	maximum int
}

func (b *boundedJSONBuffer) write(value []byte) error {
	if len(value) > b.maximum-b.Len() {
		return fmt.Errorf("final Workspace authority exceeds %d bytes", b.maximum)
	}
	_, err := b.Buffer.Write(value)
	return err
}

func (b *boundedJSONBuffer) literal(value string) error { return b.write([]byte(value)) }

func writeJSONValue(output *boundedJSONBuffer, value reflect.Value) error { //nolint:gocyclo // closed JSON kinds are easier to audit in one encoder.
	if !value.IsValid() {
		return output.literal("null")
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return output.literal("null")
		}
		return writeJSONValue(output, value.Elem())
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return output.literal("null")
		}
		return writeJSONValue(output, value.Elem())
	}
	if value.CanInterface() {
		if marshaler, ok := value.Interface().(json.Marshaler); ok {
			encoded, err := marshaler.MarshalJSON()
			if err != nil {
				return err
			}
			return output.write(encoded)
		}
	}
	switch value.Kind() {
	case reflect.Bool:
		return output.literal(strconv.FormatBool(value.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return output.literal(strconv.FormatInt(value.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return output.literal(strconv.FormatUint(value.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		number := value.Float()
		if math.IsInf(number, 0) || math.IsNaN(number) {
			return fmt.Errorf("unsupported JSON number")
		}
		return output.literal(strconv.FormatFloat(number, 'g', -1, value.Type().Bits()))
	case reflect.String:
		encoded, err := json.Marshal(value.String())
		if err != nil {
			return err
		}
		return output.write(encoded)
	case reflect.Slice:
		if value.IsNil() {
			return output.literal("null")
		}
		if value.Type().Elem().Kind() == reflect.Uint8 {
			if err := output.literal(`"`); err != nil {
				return err
			}
			encoded := base64.StdEncoding.EncodeToString(value.Bytes())
			if err := output.literal(encoded); err != nil {
				return err
			}
			return output.literal(`"`)
		}
		fallthrough
	case reflect.Array:
		if err := output.literal("["); err != nil {
			return err
		}
		for index := 0; index < value.Len(); index++ {
			if index > 0 {
				if err := output.literal(","); err != nil {
					return err
				}
			}
			if err := writeJSONValue(output, value.Index(index)); err != nil {
				return err
			}
		}
		return output.literal("]")
	case reflect.Struct:
		return writeJSONObject(output, value)
	case reflect.Map:
		return writeJSONMap(output, value)
	default:
		return fmt.Errorf("unsupported JSON kind %s", value.Kind())
	}
}

func writeJSONObject(output *boundedJSONBuffer, value reflect.Value) error {
	if err := output.literal("{"); err != nil {
		return err
	}
	written := false
	if err := writeJSONFields(output, value, &written); err != nil {
		return err
	}
	return output.literal("}")
}

func writeJSONFields(output *boundedJSONBuffer, value reflect.Value, written *bool) error {
	typeInfo := value.Type()
	for index := 0; index < value.NumField(); index++ {
		fieldInfo := typeInfo.Field(index)
		if fieldInfo.PkgPath != "" {
			continue
		}
		tag := fieldInfo.Tag.Get("json")
		name, options, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		field := value.Field(index)
		if fieldInfo.Anonymous && tag == "" {
			for field.Kind() == reflect.Pointer {
				if field.IsNil() {
					break
				}
				field = field.Elem()
			}
			if field.Kind() == reflect.Struct {
				if err := writeJSONFields(output, field, written); err != nil {
					return err
				}
				continue
			}
		}
		if name == "" {
			name = fieldInfo.Name
		}
		if strings.Contains(options, "omitempty") && emptyJSONValue(field) {
			continue
		}
		if *written {
			if err := output.literal(","); err != nil {
				return err
			}
		}
		encodedName, err := json.Marshal(name)
		if err != nil {
			return err
		}
		if err := output.write(encodedName); err != nil {
			return err
		}
		if err := output.literal(":"); err != nil {
			return err
		}
		if err := writeJSONValue(output, field); err != nil {
			return err
		}
		*written = true
	}
	return nil
}

func writeJSONMap(output *boundedJSONBuffer, value reflect.Value) error {
	if value.IsNil() {
		return output.literal("null")
	}
	if value.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("unsupported JSON map key %s", value.Type().Key())
	}
	keys := value.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	if err := output.literal("{"); err != nil {
		return err
	}
	for index, key := range keys {
		if index > 0 {
			if err := output.literal(","); err != nil {
				return err
			}
		}
		encodedKey, err := json.Marshal(key.String())
		if err != nil {
			return err
		}
		if err := output.write(encodedKey); err != nil {
			return err
		}
		if err := output.literal(":"); err != nil {
			return err
		}
		if err := writeJSONValue(output, value.MapIndex(key)); err != nil {
			return err
		}
	}
	return output.literal("}")
}

func emptyJSONValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Bool:
		return !value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return value.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return value.IsNil()
	}
	return false
}
