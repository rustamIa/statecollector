package fileutil

import (
	"fmt"
	"io"
	"os"
)

/* Если файл отсутствует или пуст, в консоль соответствующее сообщение.
Для получения размера файла  метод Stat(), который возвращает информацию о файле и ошибку.
*/

var FileOpener = Openfile

// Openfile opens a file and check it size.
// If the file does not exist or is empty, an appropriate message is printed to the console.
// To get the size of the file, the Stat() method is used, which returns information about the file and an error.
func Openfile(fileName string) (result []byte, err error) {

	//сначала проверяем что файл существует
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	//статус файла
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	sizeFile := fileInfo.Size() //кол-во байт в файле

	//файл не пустой
	if sizeFile == 0 {
		return nil, fmt.Errorf("opening file is empty")
	} else {

		result, err = io.ReadAll(file)

		if err != nil {
			return nil, err
		}

		return
	}
}
