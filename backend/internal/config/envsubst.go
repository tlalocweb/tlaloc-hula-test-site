package config

import (
	"bufio"
	"os"
	"reflect"
	"regexp"
	"strings"
)

var envPattern = regexp.MustCompile(`\{\{env:([^}]+)\}\}`)

// loadDotEnv reads a .env file and sets the variables in the process environment.
// Missing file is not an error. Lines starting with # are comments.
// Supports KEY=VALUE, KEY="VALUE", KEY='VALUE', and export KEY=VALUE.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Strip matching quotes
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		os.Setenv(key, val)
	}
	return scanner.Err()
}

// substEnvVars replaces all {{env:VAR}} patterns in a string with os.Getenv(VAR).
func substEnvVars(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(match string) string {
		m := envPattern.FindStringSubmatch(match)
		if len(m) == 2 {
			if val := os.Getenv(m[1]); val != "" {
				return val
			}
		}
		return match // leave unresolved vars as-is
	})
}

// walkAndSubstStrings recursively walks a struct and applies env var substitution
// to all string fields that contain {{env:*}} patterns.
func walkAndSubstStrings(v reflect.Value) {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanSet() {
				continue
			}
			walkAndSubstStrings(f)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkAndSubstStrings(v.Index(i))
		}
	case reflect.String:
		if v.CanSet() {
			orig := v.String()
			if strings.Contains(orig, "{{env:") {
				v.SetString(substEnvVars(orig))
			}
		}
	}
}
