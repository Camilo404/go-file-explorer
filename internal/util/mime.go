package util

import (
	"net/http"
	"os"
)

func DetectMIMEFromFile(file *os.File) (string, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil {
		return "", err
	}

	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}

	return http.DetectContentType(buffer[:n]), nil
}
