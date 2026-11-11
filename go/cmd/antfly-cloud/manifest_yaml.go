package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"gopkg.in/yaml.v3"
)

// Decode through nodes so YAML's float64 resolution never rounds an opaque
// numeric value. Bound alias expansion and reject duplicate/non-string keys,
// while retaining YAML anchors and merge-key precedence.
func sourceManifestYAMLToJSON(raw []byte) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("expected exactly one YAML document")
		}
		return nil, err
	}
	conversion := manifestYAMLDecoder{remaining: 1 << 20, active: make(map[*yaml.Node]bool)}
	value, err := conversion.value(&document, 0)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

type manifestYAMLDecoder struct {
	remaining int
	active    map[*yaml.Node]bool
}

func (d *manifestYAMLDecoder) value(node *yaml.Node, depth int) (any, error) {
	d.remaining -= len(node.Value) + 1
	if d.remaining < 0 || depth > 128 {
		return nil, errors.New("expanded YAML manifest exceeds size or nesting budget")
	}
	if d.active[node] {
		return nil, errors.New("cyclic YAML alias")
	}
	d.active[node] = true
	defer delete(d.active, node)
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			return nil, errors.New("expected one manifest document")
		}
		return d.value(node.Content[0], depth+1)
	case yaml.AliasNode:
		return d.value(node.Alias, depth+1)
	case yaml.SequenceNode:
		result := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := d.value(child, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		return result, nil
	case yaml.MappingNode:
		result := make(map[string]any)
		explicit := make(map[string]any)
		seen := make(map[string]bool)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			merge := key.Tag == "!!merge"
			var name string
			if merge {
				name = "<<"
			} else {
				value, err := d.value(key, depth+1)
				if err != nil {
					return nil, err
				}
				var ok bool
				name, ok = value.(string)
				if !ok {
					return nil, fmt.Errorf("non-string YAML map key on line %d", key.Line)
				}
			}
			identity := name
			if merge {
				identity = "merge:" + name
			} else {
				identity = "string:" + name
			}
			if seen[identity] {
				return nil, fmt.Errorf("duplicate YAML map key %q on line %d", name, key.Line)
			}
			seen[identity] = true
			value, err := d.value(node.Content[i+1], depth+1)
			if err != nil {
				return nil, err
			}
			if !merge {
				explicit[name] = value
				continue
			}
			maps := []any{value}
			if sequence, ok := value.([]any); ok {
				maps = sequence
			}
			for _, value := range maps {
				inherited, ok := value.(map[string]any)
				if !ok {
					return nil, errors.New("YAML merge requires mappings")
				}
				for name, child := range inherited {
					if _, exists := result[name]; !exists {
						result[name] = child
					}
				}
			}
		}
		for name, value := range explicit {
			result[name] = value
		}
		return result, nil
	case yaml.ScalarNode:
		if node.Tag == "!!int" || node.Tag == "!!float" {
			return manifestYAMLNumber(node.Value, node.Tag)
		}
		// The YAML resolver labels out-of-float64-range plain numbers as
		// strings. Recognize decimal syntax ourselves, retaining explicit and
		// quoted string tags.
		if node.Tag == "!!str" && node.Style == 0 {
			if number, err := manifestYAMLNumber(node.Value, "!!float"); err == nil {
				return number, nil
			}
		}
		var value any
		if err := node.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, errors.New("unsupported YAML node")
	}
}
func manifestYAMLNumber(value, tag string) (json.Number, error) {
	text := strings.ReplaceAll(value, "_", "")
	if tag == "!!int" {
		if integer, ok := new(big.Int).SetString(text, 0); ok {
			return json.Number(integer.String()), nil
		}
	}
	// Normalize YAML decimal notation without converting to binary floating point.
	text = strings.TrimPrefix(text, "+")
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign = "-"
		text = text[1:]
	}
	mantissa, exponent, hasExponent := strings.Cut(strings.ToLower(text), "e")
	if !strings.ContainsAny(mantissa, "0123456789") {
		return "", fmt.Errorf("invalid YAML number %q", value)
	}
	integer, fraction, hasPoint := strings.Cut(mantissa, ".")
	integer = strings.TrimLeft(integer, "0")
	if integer == "" {
		integer = "0"
	}
	text = sign + integer
	if hasPoint {
		if fraction == "" {
			fraction = "0"
		}
		text += "." + fraction
	}
	if hasExponent {
		text += "e" + exponent
	}
	number := json.Number(text)
	if _, err := json.Marshal(number); err != nil {
		return "", fmt.Errorf("invalid or non-finite YAML number %q", value)
	}
	return number, nil
}
