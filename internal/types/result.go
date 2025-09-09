package types

import (
	"encoding/json"
	"fmt"
	"log"
	bill "main/billingstat"
	email "main/emaildata"
	incid "main/incidentdata"
	mms "main/mmsdata"
	sms "main/smsdata"
	supp "main/support"
	voice "main/voicedata"
)

type ResultT struct {
	Status bool       `json:"status"` // true, если все этапы сбора данных  прошли успешно, false во всех остальных случаях
	Data   ResultSetT `json:"data"`   // заполнен, если все этапы сбора данных прошли успешно, nil во всех остальных случаях
	Error  string     `json:"error"`  // пустая строка если все этапы сбора данных прошли успешно, в случае ошибки заполнено текстом ошибки (детали ниже)
}

type ResultSetT struct {
	SMS       [][]sms.SMSData                `json:"sms"`
	MMS       [][]mms.MMSData                `json:"mms"`
	VoiceCall []voice.VoiceCallData          `json:"voice_call"`
	Email     map[string][][]email.EmailData `json:"email"`
	Billing   bill.BillingData               `json:"billing"`
	Support   []supp.SupportData             `json:"support"` //Support   []int                          `json:”support”`
	Incidents []incid.IncidentData           `json:"incident"`
}

func PrepaireResStub() {

	smsTest1 := [][]sms.SMSData{
		{
			{Country: "US", Bandwidth: "64", ResponseTime: "1923", Provider: "Rond"},
		},
		{
			{Country: "GB", Bandwidth: "88", ResponseTime: "1892", Provider: "Topolo"},
		},
		{
			{Country: "FR", Bandwidth: "61", ResponseTime: "170", Provider: "Topolo"},
		},
		{
			{Country: "BL", Bandwidth: "57", ResponseTime: "267", Provider: "Kildy"},
		},
	}
	mmsTest1 := [][]mms.MMSData{
		{
			{Country: "US", Provider: "Rond", Bandwidth: "36", ResponseTime: "1576"},
		},
		{
			{Country: "GB", Provider: "Kildy", Bandwidth: "85", ResponseTime: "300"},
		},
	}
	voiceTest1 := []voice.VoiceCallData{
		{
			Country:             "RU",
			Bandwidth:           "86",
			ResponseTime:        "297",
			Provider:            "TransparentCalls",
			ConnectionStability: 0.9,
			TTFB:                120,
			VoicePurity:         80,
			MedianOfCallsTime:   30,
		},
		{
			Country:             "US",
			Bandwidth:           "64",
			ResponseTime:        "1923",
			Provider:            "E-Voice",
			ConnectionStability: 0.75,
			TTFB:                200,
			VoicePurity:         65,
			MedianOfCallsTime:   45,
		},
		{
			Country:             "GB",
			Bandwidth:           "88",
			ResponseTime:        "1892",
			Provider:            "JustPhone",
			ConnectionStability: 0.6,
			TTFB:                150,
			VoicePurity:         70,
			MedianOfCallsTime:   50,
		},
	}

	emailTest1 := map[string][][]email.EmailData{
		"RU": {
			{
				{Country: "RU", Provider: "Gmail", DeliveryTime: 23},
			},
			{
				{Country: "RU", Provider: "Yahoo", DeliveryTime: 169},
			},
			{
				{Country: "RU", Provider: "Hotmail", DeliveryTime: 63},
			},
			{
				{Country: "RU", Provider: "MSN", DeliveryTime: 475},
			},
			{
				{Country: "RU", Provider: "Orange", DeliveryTime: 519},
			},
			{
				{Country: "RU", Provider: "Comcast", DeliveryTime: 408},
			},
			{
				{Country: "RU", Provider: "AOL", DeliveryTime: 254},
			},
			{
				{Country: "RU", Provider: "GMX", DeliveryTime: 246},
			},
		},
	}
	/*	billTest1 := bill.BillingData{CreateCustomer: true, Purchase: false, Payout: true, Recurring: false, FraudControl: true, CheckoutPage: false}
		suppTest1 := supp.SupportData{Topic: "issue of everything", ActiveTickets: 1}
		incidTest1 := incid.IncidentData{Topic: "boom", Status: "active"}*/

	// 2. Создание и заполнение структуры ResultSetT
	resultSet := ResultSetT{
		SMS:       smsTest1,
		MMS:       mmsTest1,
		VoiceCall: voiceTest1,
		Email:     emailTest1,
		//	Billing:   billingData,
		//		Support:   supportData,
		//		Incidents: incidentsData,
	}
	// 3. Создание и заполнение главной структуры ResultT
	/*result := ResultT{
		Status: true,
		Data:   resultSet,
		Error:  "",
	}*/

	// 4. Маршалинг всей структуры ResultT в JSON
	jsonData, err := json.Marshal(resultSet)
	if err != nil {
		log.Fatalf("Ошибка маршалинга: %v", err)
	}

	// 5. Вывод JSON в консоль
	fmt.Println(string(jsonData))

}
