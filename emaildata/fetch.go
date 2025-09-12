package emaildata

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
Считываем Email.data file
проверка строк на соответствие:
	4. Каждая строка должна содержать 3 полей (alpha-2 код страны, провайдер, стабильность
	соединения). Строки	содержащие отличное количество полей не должны попадать в результат
	работы функции.
	5. Некоторые строки могут быть повреждены, их нужно пропускать и не
	записывать в результат выполнения функции
	6. В результат допускаются только страны прошедшие проверку на
	существование по alpha-2 коду.
	7. В результат допускаются только корректные провайдеры. Все некорректные
	провайдеры нужно пропускать и не добавлять в результат работы
	функции
	8. Строки в которых меньше 3-х полей данных не допускаются
	9. Все целочисленные данные должны быть приведены к типу int
*/

func Fetch(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) ([]m.EmailData, error) {

	// файл c voice
	path := cfg.FileEmail

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
	lines := strings.Split(string(rf), "\n")

	out := make([]m.EmailData, 0, len(lines)) //данные будут срезе, len = , cap =	//так чтобы не было пустых элементов в срезе, cap сразу чтоб не переназначалась каждый раз память

	for _, line := range lines {
		splitted, ok := textutil.SplitN(line, ';', cfg.QuantEmailDataCol) //критерий 5,8 //перешли на более дешевый метод SplitN.
		if !ok {
			continue
		}

		DeliveryTime, err := strconv.Atoi(splitted[2])
		//проверка на соответствие критерия 9 - поле не цифра
		if err != nil {
			continue
		}

		//заполняем структуру провайдера
		e := m.EmailData{
			Country:      splitted[0],
			Provider:     splitted[1],
			DeliveryTime: DeliveryTime,
		}

		if err := e.Validate(); err == nil { //проверка на соответствие критериям 4, 6, 7
			out = append(out, e)
		}

	}
	return out, nil

}
