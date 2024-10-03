// Copyright (c) ClaceIO, LLC
// SPDX-License-Identifier: Apache-2.0

package apptype

import (
	"fmt"

	"go.starlark.net/starlark"
)

func GetStringAttr(s starlark.HasAttrs, key string) (string, error) {
	v, err := s.Attr(key)
	if err != nil {
		return "", fmt.Errorf("error getting %s: %s", key, err)
	}
	var vs starlark.String
	var ok bool
	if vs, ok = v.(starlark.String); !ok {
		return "", fmt.Errorf("%s is not a string", key)
	}
	return vs.GoString(), nil
}

func GetIntAttr(s starlark.HasAttrs, key string) (int64, error) {
	v, err := s.Attr(key)
	if err != nil {
		return 0, fmt.Errorf("error getting %s: %s", key, err)
	}
	var vi starlark.Int
	var ok bool
	if vi, ok = v.(starlark.Int); !ok {
		return 0, fmt.Errorf("%s is not a integer", key)
	}
	intVal, ok := vi.Int64()
	if !ok {
		return 0, fmt.Errorf("%s is not a integer", key)
	}
	return intVal, nil
}

func GetBoolAttr(s starlark.HasAttrs, key string) (bool, error) {
	v, err := s.Attr(key)
	if err != nil {
		return false, fmt.Errorf("error getting %s: %s", key, err)
	}
	var vb starlark.Bool
	var ok bool
	if vb, ok = v.(starlark.Bool); !ok {
		return false, fmt.Errorf("%s is not a bool", key)
	}
	return bool(vb), nil
}

func GetListStringAttr(s starlark.HasAttrs, key string, optional bool) ([]string, error) {
	v, err := s.Attr(key)
	if err != nil {
		if optional {
			return []string{}, nil
		} else {
			return nil, fmt.Errorf("error getting %s: %s", key, err)
		}
	}
	var list *starlark.List
	var ok bool
	if list, ok = v.(*starlark.List); !ok {
		return nil, fmt.Errorf("%s is not a list", key)
	}

	return GetStringList(list)
}

func GetStringList(list *starlark.List) ([]string, error) {
	ret := []string{}
	iter := list.Iterate()
	var ok bool
	var val starlark.Value
	var vs starlark.String
	count := -1
	for iter.Next(&val) {
		count++
		if vs, ok = val.(starlark.String); !ok {
			return nil, fmt.Errorf("entry %d in list is not a string", count)
		}
		ret = append(ret, string(vs))
	}

	return ret, nil
}

func GetCallableAttr(s starlark.HasAttrs, key string) (starlark.Callable, error) {
	v, err := s.Attr(key)
	if err != nil {
		return nil, fmt.Errorf("error getting %s: %s", key, err)
	}
	var vc starlark.Callable
	var ok bool
	if vc, ok = v.(starlark.Callable); !ok {
		return nil, fmt.Errorf("%s is not a callable", key)
	}
	return vc, nil
}
