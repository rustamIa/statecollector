package validatestruct

import (
	"strconv"

	"github.com/go-playground/validator/v10"
)

// общий валидатор (инициализируется один раз)
var v = validator.New()

// поле SMSData.Bandwidth должно остаться string, придётся зарегистрировать свою проверку: встроенные min/max
func init() {
	v.RegisterValidation("num0to100", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		n, err := strconv.Atoi(s)
		return err == nil && n >= 0 && n <= 100
	})
}

// Struct — функция если нужно валидироват объект
func Struct(s any) error {
	// объект валидатора - передаем в него структуру, которую нужно провалидировать
	return v.Struct(s)
}

// columnsCorrect determines if the columns of an  data  are correct by columns quantity
//
// Parameters:
// DataLine - a slice of strings containing the data from an data record
//
// Returns:
// true if the columns of the SMS data record are correct, false otherwise
func ColumnsChecker(SMSDataLine []string, quantCol int) bool {
	return len(SMSDataLine) == quantCol
}

// TODO: это не используется
func ValidateStruct(s any) (bool, error) {
	// Создаем объект валидатора
	// и передаем в него структуру, которую нужно провалидировать
	if err := validator.New().Struct(s); err != nil {
		// Приводим ошибку к типу ошибки валидации
		//validateErr := err.(validator.ValidationErrors)
		return false, err
	}
	return true, nil
}
