package misc

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
)

func ReadFile(fileName string) (error, []byte) {
	if fileName == "" {
		return errors.New("no filename supplied"), []byte{}
	}
	// open file for reading
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("unable to open %s - %s", fileName, err), []byte{}
	}
	// read contents from open file
	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return fmt.Errorf("unable to read %s - %s", fileName, err), []byte{}
	}
	// close file
	err = file.Close()
	if err != nil {
		return fmt.Errorf("unable to close %s - %s", fileName, err), []byte{}
	}

	return nil, fileBytes
}

func WriteFile(fileName string, contents []byte) (int, error) {
	if fileName == "" {
		return 0, errors.New("no filename supplied")
	}
	// create/truncate file for writing
	file, err := os.Create(fileName)
	if err != nil {
		return 0, fmt.Errorf("unable to create file %s - %s", fileName, err)
	}
	// write contents to open file
	bytesWritten, err := file.Write(contents)
	if err != nil {
		return bytesWritten, fmt.Errorf("unable to write file %s - %s", fileName, err)
	}
	// close file
	err = file.Close()
	if err != nil {
		return bytesWritten, fmt.Errorf("unable to close file %s - %s", fileName, err)
	}

	return bytesWritten, nil
}
