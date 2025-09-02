package emaildata

import (
	"log/slog"
	"main/config"
	"main/internal/fileutil"
	"main/internal/textutil"
	"main/internal/validateStruct"
	"main/sl"
	"strconv"
	"strings"
)

// структура для SMSData
type EmailData struct {
	Country      string `validate:"iso3166_1_alpha2"`
	Provider     string `validate:"oneof=Gmail Yahoo Hotmail MSN Orange Comcast AOL Live RediffMail GMX Protonmail Yandex Mail.ru"`
	DeliveryTime int    `validate:"required"`
}

// Вызов метода валидации структуры
func (v EmailData) Validate() error {
	return validateStruct.Struct(v)
}

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

func Fetch(logger *slog.Logger, cfg *config.CfgApp) ([]EmailData, error) {

	// файл c voice
	fileEmail := cfg.FileEmail
	rf, err := fileutil.FileOpener(fileEmail)

	if err != nil {
		logger.Error("Error by opening file "+fileEmail, sl.Err(err))
		return nil, err
	}

	//преобразовать байты в массив строк, разделитель новая строка, затем разделитель ;
	EmailDataLines := strings.Split(string(rf), "\n")

	EmailDatas := make([]EmailData, 0, len(EmailDataLines)) //данные будут срезе, len = , cap =	//так чтобы не было пустых элементов в срезе, cap сразу чтоб не переназначалась каждый раз память

	for _, line := range EmailDataLines {
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
		e := EmailData{
			Country:      splitted[0],
			Provider:     splitted[1],
			DeliveryTime: DeliveryTime,
		}

		if err := e.Validate(); err == nil { //проверка на соответствие критериям 4, 6, 7
			EmailDatas = append(EmailDatas, e)
		}

	}
	return EmailDatas, nil

}
