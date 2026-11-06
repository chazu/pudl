package populate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"

	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/systemmodel"
)

// maxPages bounds an enumeration that keeps returning continuation tokens.
const maxPages = 1000

// enumerate pages through the `each` list and returns its inputs, in order,
// without duplicates. Any failure fails the populate.
func (c *httpSource) enumerate(each systemmodel.HTTPEnumeration) ([]string, error) {
	listPath, err := fieldpath.Parse(each.List)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	var nextPath fieldpath.Path
	if each.Page != nil {
		if nextPath, err = fieldpath.Parse(each.Page.Next); err != nil {
			return nil, fmt.Errorf("page.next: %w", err)
		}
	}
	items := make([]string, 0)
	seen, tokens := map[string]bool{}, map[string]bool{}
	path := each.Get
	for page := 0; ; page++ {
		if page == maxPages {
			return nil, fmt.Errorf("more than %d pages", maxPages)
		}
		body, err := c.get(path)
		if err != nil {
			return nil, err
		}
		obj, ok := body.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("response is %s, not a JSON object", jsonKind(body))
		}
		listed, err := lookupArray(obj, listPath)
		if err != nil {
			return nil, err
		}
		for _, element := range listed {
			item, err := listedItem(each.Item, element, len(items))
			if err != nil {
				return nil, err
			}
			if err := systemmodel.ValidateItem(item); err != nil {
				return nil, err
			}
			if !seen[item] {
				seen[item] = true
				items = append(items, item)
			}
		}
		if each.Page == nil {
			return items, nil
		}
		token, _ := lookupString(obj, nextPath)
		if token == "" {
			return items, nil
		}
		if tokens[token] {
			return nil, errors.New("the continuation token repeats")
		}
		tokens[token] = true
		if path, err = withQuery(each.Get, each.Page.Param, token); err != nil {
			return nil, err
		}
	}
}

// listedItem is one listed element's input: the item mapping's string, or the
// element itself when it is a string.
func listedItem(mapping *systemmodel.Transform, element any, index int) (string, error) {
	if mapping != nil {
		item, err := mapping.String(element)
		if err != nil {
			return "", fmt.Errorf("element %d: item could not be evaluated%s", index, systemmodel.ErrorPaths(err))
		}
		return item, nil
	}
	switch v := element.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	}
	return "", fmt.Errorf("element %d is %s, not a string (set item to map it)", index, jsonKind(element))
}

// observe sends one run's GET and returns its records, transformed and with
// set applied. A failed request or a bad record is the run's failed input; a
// record that keeps a rejected key fails the populate.
func (c *httpSource) observe(run systemmodel.HTTPRun) ([]any, *FailedInput, error) {
	fail := func(err error) ([]any, *FailedInput, error) {
		return nil, &FailedInput{ExitStatus: -1, Error: err.Error()}, nil
	}
	body, err := c.get(run.Get)
	if errors.Is(err, errHTTPPopulate) {
		return nil, nil, err
	}
	if err != nil {
		return fail(err)
	}
	listed, err := runRecords(body, run.Records)
	if err != nil {
		return fail(err)
	}
	assignments := flattenSet(nil, run.Set)
	records := make([]any, 0, len(listed))
	for i, v := range listed {
		obj, ok := v.(map[string]any)
		if !ok {
			return fail(fmt.Errorf("record %d is %s, not a JSON object", i, jsonKind(v)))
		}
		if run.Transform != nil {
			out, keep, err := run.Transform.Apply(obj)
			if err != nil {
				// CUE messages quote values, which may be secrets: name paths.
				return fail(fmt.Errorf("record %d: transform failed%s", i, systemmodel.ErrorPaths(err)))
			}
			if !keep {
				continue
			}
			obj = out
		}
		for _, a := range assignments {
			if err := a.path.Set(obj, a.value); err != nil {
				return fail(fmt.Errorf("set: %w", err))
			}
		}
		if key, found := rejectedKey(obj, c.spec.RejectKeys); found {
			return nil, nil, fmt.Errorf("record %d still has rejected key %q after the transform; nothing is ingested", i, key)
		}
		records = append(records, obj)
	}
	return records, nil, nil
}

// runRecords selects a response's records: the array at path, or without a
// path the response itself (one object, or an array). null at path is no
// records; a missing path is an error, so an API that changes its shape is
// not read as "nothing here".
func runRecords(body any, path string) ([]any, error) {
	if path == "" {
		if list, ok := body.([]any); ok {
			return list, nil
		}
		return []any{body}, nil
	}
	p, err := fieldpath.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("records: %w", err)
	}
	obj, ok := body.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("response is %s, not a JSON object", jsonKind(body))
	}
	return lookupArray(obj, p)
}

func lookupArray(obj map[string]any, p fieldpath.Path) ([]any, error) {
	v, ok := p.Lookup(obj)
	if !ok {
		return nil, fmt.Errorf("response has no %s", p)
	}
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("response %s is %s, not an array", p, jsonKind(v))
	}
	return list, nil
}

func lookupString(obj map[string]any, p fieldpath.Path) (string, bool) {
	v, ok := p.Lookup(obj)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// withQuery sets one query parameter on a request path.
func withQuery(path, param, value string) (string, error) {
	u, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set(param, value)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// rejectedKey finds a key of reject at any depth of v, sorted so the first
// found is stable.
func rejectedKey(v any, reject []string) (string, bool) {
	if len(reject) == 0 {
		return "", false
	}
	switch v := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			for _, r := range reject {
				if k == r {
					return k, true
				}
			}
			if key, found := rejectedKey(v[k], reject); found {
				return key, true
			}
		}
	case []any:
		for _, child := range v {
			if key, found := rejectedKey(child, reject); found {
				return key, true
			}
		}
	}
	return "", false
}

// jsonKind names a decoded JSON value's kind without its content.
func jsonKind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "an object"
	case []any:
		return "an array"
	case string:
		return "a string"
	case json.Number:
		return "a number"
	case bool:
		return "a boolean"
	}
	return fmt.Sprintf("%T", v)
}
