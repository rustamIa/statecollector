package voicedata

import (
	"context"
	"log/slog"
	"main/config"
	"main/internal/fileutil"
	m "main/internal/model"
	"main/internal/textutil"
	"main/sl"
	"strconv"
	"strings"
)

/*
Считываем Voice.data file
проверка строк на соответствие:
	4. Каждая строка должна содержать 8 полей (alpha-2 код страны, текущая
	нагрузка в процентах, среднее время ответа, провайдер, стабильность
	соединения, TTFB, чистота связи, медиана длительности звонка). Строки
	содержащие отличное количество полей не должны попадать в результат
	работы функции.
	5. Некоторые строки могут быть повреждены, их нужно пропускать и не
	записывать в результат выполнения функции
	6. В результат допускаются только страны прошедшие проверку на
	существование по alpha-2 коду.
	7. В результат допускаются только корректные провайдеры. Допустимые
	провайдеры: TransparentCalls, E-Voice, JustPhone. Все некорректные
	провайдеры нужно пропускать и не добавлять в результат работы
	функции
	8. Строки в которых меньше 8-х полей данных не допускаются
	9. Все целочисленные данные должны быть приведены к типу int
	10.Все числа с плавающей точкой должны быть приведены к типу float32
*/
//go:generate go run github.com/vektra/mockery/v2@v2.28.2 --name=readfile
func Fetch(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) ([]m.VoiceCallData, error) {

	// файл c voice
	path := cfg.FileVoiceCall
	rf, err := fileutil.FileOpener(path)

	if err != nil {
		logger.Error("Error by opening file "+path, sl.Err(err))
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
	VoiceDataLines := strings.Split(string(rf), "\n")

	VoiceDatas := make([]m.VoiceCallData, 0, len(VoiceDataLines)) //данные будут срезе, len = , cap =	//так чтобы не было пустых элементов в срезе, cap сразу чтоб не переназначалась каждый раз память

	for _, line := range VoiceDataLines {
		splitted, ok := textutil.SplitN(line, ';', cfg.QuantVoiceDataCol) //перешли на более дешевый метод SplitN.
		if !ok {
			continue
		}

		ConnectionStability, err := strconv.ParseFloat(splitted[4], 32)
		//проверка на соответствие критерия 5 - поле не цифра
		if err != nil {
			continue
		}
		TTFB, err := strconv.Atoi(splitted[5])
		//проверка на соответствие критерия 5 - поле не цифра
		if err != nil {
			continue
		}
		VoicePurity, err := strconv.Atoi(splitted[6])
		//проверка на соответствие критерия 5 - поле не цифра
		if err != nil {
			continue
		}
		MedianOfCallsTime, err := strconv.Atoi(splitted[7])
		//проверка на соответствие критерия 5 - поле не цифра
		if err != nil {
			continue
		}

		//заполняем структуру провайдера
		s := m.VoiceCallData{
			Country:             splitted[0],
			Bandwidth:           splitted[1],
			ResponseTime:        splitted[2],
			Provider:            splitted[3],
			ConnectionStability: float32(ConnectionStability),
			TTFB:                TTFB,
			VoicePurity:         VoicePurity,
			MedianOfCallsTime:   MedianOfCallsTime,
		}

		if err := s.Validate(); err == nil { //проверка на соответствие критериям 4, 6, 7
			VoiceDatas = append(VoiceDatas, s)
		}

	}

	return VoiceDatas, nil

}
