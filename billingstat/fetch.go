package billingstat

import (
	"context"
	"fmt"
	"log/slog"
	"main/config"
	"main/internal/fileutil"
	m "main/internal/model"
	"main/sl"
	"reflect"
)

//go:generate go run github.com/vektra/mockery/v2@v2.28.2 --name=readfile
func Fetch(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) (m.BillingData, error) {

	// файл c voice
	path := cfg.FileBillingState
	rf, err := fileutil.FileOpener(path)
	bd := &m.BillingData{}

	if err != nil {
		logger.Error("Error by opening file "+path, sl.Err(err))
		return *bd, err
	}

	/*Почему останавливаемся тут
	Ранний выход без «публикации». Даже если парсинг и валидация быстрые, по отмене лучше вернуть ошибку и не делать больше ничего.
	Тогда вызывающий код (горутина) не будет логировать “fetched” и не будет публиковать результат.
	*/
	if err := ctx.Err(); err != nil {
		return *bd, err
	}

	err = decodeBinaryState(rf, bd)
	if err != nil {
		logger.Error("Error by decoding Billing binary state", sl.Err(err))
		return *bd, err
	}

	return *bd, nil
}

func decodeBinaryState(txt []byte, bd *m.BillingData) (err error) {
	if len(txt) == 0 {
		err := fmt.Errorf("empty file")
		return err
	}

	sd, err := getStateDec(txt)
	if err != nil {
		return err
	}

	bd.CreateCustomer = (sd & 1) != 0 //1-й бит
	bd.Purchase = (sd & 2) != 0       //2-й бит
	bd.Payout = (sd & 4) != 0         //3-й бит
	bd.Recurring = (sd & 8) != 0      //4-й бит
	bd.FraudControl = (sd & 16) != 0  //5-й бит
	bd.CheckoutPage = (sd & 32) != 0  //6-й бит

	return nil
}

func byteToBool(b byte) (bool, error) {
	switch b {
	case '1':
		return true, nil
	case '0':
		return false, nil
	default:
		// Обработка ошибки для некорректных значений
		return false, fmt.Errorf("error by converting string state to bool: invalid byte '%c'", b)
	}
}

// Функция для возведения 2 в степень
func powerOfTwo(exp int) uint8 {
	return 1 << exp // сдвиг на exp бит влево
}

func getStateDec(s []byte) (dec uint8, err error) {
	quanBits := reflect.TypeOf(m.BillingData{}).NumField() //по заданию полей 6. а можно задать так: bdType := reflect.TypeOf(m.BillingData{}) numFields := bdType.NumField()

	j := 0

	for i := quanBits - 1; i >= 0; i-- {

		b, err := byteToBool(s[i])

		if err != nil {
			return dec, err
		}

		if b {
			dec += powerOfTwo(j)
		}
		j++
	}
	return dec, nil
}
