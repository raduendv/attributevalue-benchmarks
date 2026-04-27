package util

import (
	"os"
	"strconv"
)

func Env(n, dflt string) string {
	out := os.Getenv(n)
	if out == "" {
		out = dflt
	}

	return out
}

func SetEnv(n, v string) error {
	return os.Setenv(n, v)
}

func EnvInt(n string, dflt int) (int, error) {
	v := os.Getenv(n)
	if v == "" {
		return dflt, nil
	}

	return strconv.Atoi(v)
}
