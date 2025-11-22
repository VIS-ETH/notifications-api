package internal

import "os"

func EnvOrDefault(envVar, defaultVal string) string {
	envVal, exists := os.LookupEnv(envVar)
	if exists {
		return envVal
	}
	return defaultVal
}
