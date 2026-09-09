package fieldcatalog

import "fmt"

// ResolveField resolves an exact raw identity or an unambiguous display alias.
// Raw identities take precedence, including excluded fields, so callers can
// validate suitability without accidentally selecting a different field.
func ResolveField(fields []Field, selector string) (Field, error) {
	resolved, err := ResolveFields(fields, []string{selector})
	if err != nil {
		return Field{}, err
	}
	return resolved[0], nil
}

// ResolveFields indexes the catalog once and resolves selectors in input order.
// Repeated selectors remain repeated; an invalid selector fails the whole batch.
func ResolveFields(fields []Field, selectors []string) ([]Field, error) {
	if len(selectors) == 0 {
		return []Field{}, nil
	}
	raw := make(map[string]int, len(fields))
	aliases := make(map[string]int, len(fields))
	add := func(index map[string]int, key string, position int) {
		if key == "" {
			return
		}
		if _, exists := index[key]; exists {
			index[key] = -1
		} else {
			index[key] = position
		}
	}
	for i, field := range fields {
		add(raw, field.ID, i)
		add(aliases, field.Caption, i)
		if field.Label != field.Caption {
			add(aliases, field.Label, i)
		}
	}
	resolved := make([]Field, 0, len(selectors))
	for _, selector := range selectors {
		if selector == "" {
			return nil, fmt.Errorf("field selector is empty")
		}
		if i, exists := raw[selector]; exists {
			if i < 0 {
				return nil, fmt.Errorf("field %q has ambiguous raw identity", selector)
			}
			resolved = append(resolved, fields[i])
			continue
		}
		i, exists := aliases[selector]
		if !exists {
			return nil, fmt.Errorf("field %q was not found in datasource schema", selector)
		}
		if i < 0 || fields[i].ID == "" || raw[fields[i].ID] < 0 {
			return nil, fmt.Errorf("field alias %q is ambiguous; use a unique raw field name", selector)
		}
		resolved = append(resolved, fields[i])
	}
	return resolved, nil
}
