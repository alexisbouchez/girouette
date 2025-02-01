package env

import "os"

func GetVar(key, defaultValue string) string {
	if variable := os.Getenv(key); variable != "" {
		return variable
	}
	return defaultValue
}
