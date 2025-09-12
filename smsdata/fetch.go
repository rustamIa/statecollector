package smsdata

import (
	"context"
	"log/slog"
	"main/config"
	"main/internal/fileutil"
	m "main/internal/model"
	"main/internal/textutil"
	"main/sl"
	"strings"
)

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
func Fetch(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) ([]m.SMSData, error) {

	path := cfg.FileSms

	// Читаем целиком (маленький файл)
	rf, err := fileutil.FileOpener(path)
	if err != nil {
		logger.Error("Error by open/read file "+path, sl.Err(err))
		return nil, err
	}

	/*Почему останавливаемся тут
	Ранний выход без «публикации». Даже если парсинг и валидация быстрые, по отмене лучше вернуть ошибку и не делать больше ничего.
	Тогда вызывающий код (горутина) не будет логировать “fetched” и не будет публиковать результат.
	*/
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	//преобразовать байты в массив строк, разделитель новая строка, затем разделитель ;
	lines := strings.Split(string(rf), "\n")

	out := make([]m.SMSData, 0, len(lines)) //данные SMS будут срезе, len = , cap =	//так чтобы не было пустых элементов в срезе, cap сразу чтоб не переназначалась каждый раз память

	for _, line := range lines {
		splitted, ok := textutil.SplitN(line, ';', cfg.QuantSMSDataCol) //перешли на более дешевый метод SplitN. было: SMSDataLine := strings.Split(line, ";")
		if !ok {
			continue
		}
		s := m.SMSData{Country: splitted[0], Bandwidth: splitted[1], ResponseTime: splitted[2], Provider: splitted[3]}

		//if validate.ColumnsChecker(SMSDataLine, quantSMSDataCol) { //проверка на соответствие критериям 1

		if err := s.Validate(); err == nil { //проверка на соответствие критериям 2,3,4,5
			out = append(out, s)
		}

	}

	return out, nil

}
