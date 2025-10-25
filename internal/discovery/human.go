// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package discovery

import (
	"fmt"
	"io"
	"reflect"
	"strings"
)

// PrintHuman writes any struct/map/slice in a human-readable, indented form.
func PrintHuman(w io.Writer, v any, indent string) {
	printValue(w, reflect.ValueOf(v), indent)
}

func printValue(w io.Writer, v reflect.Value, indent string) {
	if !v.IsValid() {
		fmt.Fprintf(w, "%s<nil>\n", indent)
		return
	}

	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		handlePtrInterface(w, v, indent)

	case reflect.Struct:
		handleStruct(w, v, indent)

	case reflect.Map:
		handleMap(w, v, indent)

	case reflect.Slice, reflect.Array:
		handleSliceArray(w, v, indent)

	case reflect.String:
		handleString(w, v, indent)

	// explicit scalar kinds — use same behavior as default
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128:
		fmt.Fprintf(w, "%s%v\n", indent, v.Interface())

	// other reference / uncommon kinds, handle explicitly (you can format a short placeholder)
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		fmt.Fprintf(w, "%s<%s>\n", indent, v.Kind().String())

	case reflect.Invalid:
		fmt.Fprintf(w, "%s<invalid>\n", indent)

	default:
		fmt.Fprintf(w, "%s%v\n", indent, v.Interface())
	}
}

func handlePtrInterface(w io.Writer, v reflect.Value, indent string) {
	if v.IsNil() {
		fmt.Fprintf(w, "%s<nil>\n", indent)
		return
	}
	printValue(w, v.Elem(), indent)
}

func handleStruct(w io.Writer, v reflect.Value, indent string) {
	t := v.Type()
	for i := range make([]struct{}, v.NumField()) {
		field := t.Field(i)
		value := v.Field(i)
		if !value.CanInterface() {
			continue
		}
		fmt.Fprintf(w, "%s%s:\n", indent, field.Name)
		printValue(w, value, indent+"  ")
	}
}

func handleMap(w io.Writer, v reflect.Value, indent string) {
	iter := v.MapRange()
	for iter.Next() {
		k := iter.Key()
		val := iter.Value()
		fmt.Fprintf(w, "%s%v:\n", indent, k)
		printValue(w, val, indent+"  ")
	}
}

func handleSliceArray(w io.Writer, v reflect.Value, indent string) {
	if v.Len() == 0 {
		fmt.Fprintf(w, "%s[]\n", indent)
		return
	}
	for i := range make([]struct{}, v.Len()) {
		fmt.Fprintf(w, "%s- ", indent)
		printValue(w, v.Index(i), indent+"  ")
	}
}

func handleString(w io.Writer, v reflect.Value, indent string) {
	s := strings.TrimSpace(v.String())
	if s == "" {
		fmt.Fprintf(w, "%s\"\"\n", indent)
	} else {
		fmt.Fprintf(w, "%s%s\n", indent, s)
	}
}
