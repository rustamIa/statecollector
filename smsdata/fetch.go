package smsdata

import (
	"log/slog"
	"main/config"
	"main/internal/fileutil"
	"main/internal/textutil"
	"main/internal/validateStruct"
	"main/sl"
	"strings"
)

// структура для SMSData
type SMSData struct {
	Country      string `validate:"iso3166_1_alpha2"`
	Bandwidth    string `validate:"required,num0to100"` // ← только цифры (0..100)
	ResponseTime string `validate:"required,number"`    // ← в том числе float
	Provider     string `validate:"oneof=Topolo Rond Kildy"`
}

// Вызов метода валидации структуры
func (v SMSData) Validate() error {
	return validateStruct.Struct(v)
}

/*
Считываем SMS.data file
проверка строк на соответствие:
-каждая строка 4 поля: 1: код страны alpha-2; 2:пропуск способность; 3:среднее время ответа ms; 4: название компании
-строки могут быть повреждены:

	-строки в которых менее 4х или более 4х полей - пропустить ( критерий 1)
	-в результат только страны прошедшие проверку по alpha-2 коду ( критерий 2)
	-в результат только провайдеры Topolo, Rond, Kildy ( критерий 3)
	-пропускная  способность канала от 0% до 100% ( критерий 4)
	-среднее время ответа в ms ( критерий 5)

Итог переносим в SMSData struct
*/
//go:generate go run github.com/vektra/mockery/v2@v2.28.2 --name=readfile
func Fetch(logger *slog.Logger, cfg *config.CfgApp) ([]SMSData, error) {

	// файл c sms
	fileSms := cfg.FileSms

	rf, err := fileutil.FileOpener(fileSms)

	if err != nil {
		logger.Error("Error by opening file "+fileSms, sl.Err(err))
		return nil, err
	}

	//преобразовать байты в массив строк, разделитель новая строка, затем разделитель ;
	SMSDataLines := strings.Split(string(rf), "\n")

	SMSDatas := make([]SMSData, 0, len(SMSDataLines)) //данные SMS будут срезе, len = , cap =	//так чтобы не было пустых элементов в срезе, cap сразу чтоб не переназначалась каждый раз память

	for _, line := range SMSDataLines {
		splitted, ok := textutil.SplitN(line, ';', cfg.QuantSMSDataCol) //перешли на более дешевый метод SplitN. было: SMSDataLine := strings.Split(line, ";")
		if !ok {
			continue
		}
		//заполняем структуру провайдера
		s := SMSData{Country: splitted[0], Bandwidth: splitted[1], ResponseTime: splitted[2], Provider: splitted[3]}

		//if validate.ColumnsChecker(SMSDataLine, quantSMSDataCol) { //проверка на соответствие критериям 1

		if err := s.Validate(); err == nil { //проверка на соответствие критериям 2,3,4,5
			SMSDatas = append(SMSDatas, s)
		}

	}

	return SMSDatas, nil

}
