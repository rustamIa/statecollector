package fileutil

import (
	"bufio"
	"io"
	"os"
	"testing"
)

func TestReadfile(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		writeToFile bool
		wantErr     bool
	}{
		{
			name:    "absent file",
			wantErr: true,
		},
		{
			name:    "empty file",
			file:    "empty.txt",
			wantErr: true,
		},
		{
			name:        "non-empty file",
			file:        "non-empty.txt",
			writeToFile: true,
			wantErr:     false,
		},
	}

	//сами тесты
	for _, tt := range tests {
		//создаем test файлы
		if tt.file != "" {
			createTestFile(t, tt.file)
		}
		if tt.writeToFile {
			writeFile(t, tt.file, "some text data tralala")
		}
		//удаляем созданный файл
		if tt.file != "" {
			defer os.Remove(tt.file)
		}

		t.Run(tt.name, func(t *testing.T) {
			_, err := Openfile(tt.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("readfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			f, err := os.Open(tt.file)
			if err != nil {
				t.Fatalf("error opening file: %v", err)
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("error reading file: %v", err)
			}
			if len(data) == 0 {
				t.Errorf("file is empty, want non-empty")
			}
		})
	}
}

func createTestFile(t *testing.T, fileName string) {
	//создаем файл
	file, err := os.Create(fileName)
	if err != nil {
		t.Errorf("error by creating file: %v", err)
		return
	}
	if err := os.Chmod(fileName, 0777); err != nil { //0777 права на
		t.Errorf("error by creating file: %v", err)
	}
	//file.WriteString("test1*test1*test1*test1\n")
	file.Close()
}

func writeFile(t *testing.T, fileName string, text string) {
	fileWr, err := os.Create(fileName)
	writer := bufio.NewWriter(fileWr) //поток io на запись
	if err != nil {
		t.Errorf("error by creating file: %v", err)
		return
	}
	defer fileWr.Close()

	if _, err = writer.WriteString(text); err != nil { // запись строки
		t.Errorf("error by creating file: %v", err)
		return
	}
	writer.Flush() // сбрасываем данные из буфера в файл
}
