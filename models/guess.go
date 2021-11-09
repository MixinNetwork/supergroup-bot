package models

import (
	"context"
	"fmt"
	"time"

	"github.com/MixinNetwork/supergroup/session"
	"github.com/jackc/pgx/v4"
	"github.com/shopspring/decimal"
)

const guess_DDL = `
CREATE TABLE IF NOT EXISTS guess (
	client_id VARCHAR(36) NOT NULL,
	guess_id  VARCHAR(36) NOT NULL,
	asset_id VARCHAR(36) NOT NULL,
	symbol 	  VARCHAR NOT NULL,
	price_usd VARCHAR NOT NULL,
	rules 	 VARCHAR NOT NULL,
	explain VARCHAR NOT NULL,
	start_time VARCHAR NOT NULL,
	end_time VARCHAR NOT NULL,
	start_at TIMESTAMP NOT NULL,
	end_at  TIMESTAMP NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT now()
);
`

const guess_record_DDL = `
CREATE TABLE IF NOT EXISTS guess_record (
	guess_id  VARCHAR(36) NOT NULL,
	user_id VARCHAR(36) NOT NULL,
	guess_type SMALLINT NOT NULL,
	date DATE NOT NULL,
	result SMALLINT NOT NULL DEFAULT 0,
	PRIMARY KEY (guess_id, user_id, date)
);
`

const guess_result_DDL = `
CREATE TABLE IF NOT EXISTS guess_result (
	asset_id  VARCHAR(36) NOT NULL,
	price VARCHAR NOT NULL,
	date  DATE NOT NULL DEFAULT now()
);
`

type Guess struct {
	ClientId  string    `json:"client_id"`
	GuessId   string    `json:"guess_id"`
	Symbol    string    `json:"symbol"`
	AssetID   string    `json:"asset_id"`
	PriceUsd  string    `json:"price_usd"`
	Rules     string    `json:"rules"`
	Explain   string    `json:"explain"`
	StartTime string    `json:"start_time"`
	EndTime   string    `json:"end_time"`
	StartAt   time.Time `json:"start_at"`
	EndAt     time.Time `json:"end_at"`
	CreatedAt time.Time `json:"created_at"`
}

type GuessRecord struct {
	GuessId   string `json:"guess_id"`
	UserId    string `json:"user_id"`
	GuessType int    `json:"guess_type"`
	Result    int    `json:"result"`
	Date      string `json:"date"`
}

type GuessResult struct {
	AssetID string          `json:"asset_id"`
	Price   decimal.Decimal `json:"price"`
	Date    time.Time       `json:"date"`
}

const (
	GuessTypeUP = iota + 1
	GuessTypeDown
	GuessTypeFlat

	// GuessResultDraw
)

const (
	GuessResultPending = iota
	GuessResultWin
	GuessResultLose
)

type GuessPageResp struct {
	*Guess
	TodayGuess int `json:"today_guess"`
}

func GetGuessPageInitData(ctx context.Context, u *ClientUser, guessID string) (*GuessPageResp, error) {
	guess, err := getGuessByID(ctx, guessID)
	if err != nil {
		return nil, err
	}
	todyGuessType, _ := getTodayGuessType(ctx, u.UserID, guessID)
	resp := &GuessPageResp{
		Guess:      guess,
		TodayGuess: todyGuessType,
	}
	return resp, nil
}

func PostGuess(ctx context.Context, u *ClientUser, guessID string, guessType int) error {
	var count int
	session.Database(ctx).QueryRow(ctx, `
SELECT COUNT(1) FROM guess_record 
WHERE user_id = $1 AND guess_id = $2 AND date=current_date
`, u.UserID, guessID).Scan(&count)
	if count > 0 {
		return session.TooManyRequestsError(ctx)
	}
	_, err := session.Database(ctx).Exec(ctx, `
INSERT INTO guess_record (guess_id, user_id, guess_type, date, result)
VALUES ($1, $2, $3, current_date, $4)`,
		guessID, u.UserID, guessType, GuessResultPending)
	return err
}

func GetGuessRecordListByUserID(ctx context.Context, u *ClientUser, guessID string) ([]*GuessRecord, error) {
	return getGuessRecordByUserID(ctx, u.UserID, guessID)
}

func getGuessByID(ctx context.Context, guessID string) (*Guess, error) {
	var g Guess
	err := session.Database(ctx).QueryRow(ctx, `
SELECT client_id, guess_id, symbol, price_usd, rules, explain, start_time, end_time, start_at, end_at, created_at 
FROM guess 
WHERE guess_id = $1`, guessID).
		Scan(&g.ClientId, &g.GuessId, &g.Symbol, &g.PriceUsd, &g.Rules, &g.Explain, &g.StartTime, &g.EndTime, &g.StartAt, &g.EndAt, &g.CreatedAt)
	return &g, err
}

func getTodayGuessType(ctx context.Context, userID string, guessID string) (int, error) {
	var guessType int
	err := session.Database(ctx).QueryRow(ctx, `
SELECT guess_type 
FROM guess_record 
WHERE user_id = $1 AND guess_id = $2 AND date=current_date`,
		userID, guessID).Scan(&guessType)
	return guessType, err
}

func getGuessRecordByUserID(ctx context.Context, userID string, guessID string) ([]*GuessRecord, error) {
	grs := make([]*GuessRecord, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT guess_id, user_id, guess_type, to_char(date, 'YYYY-MM-DD') AS date, result 
FROM guess_record 
WHERE user_id = $1 AND guess_id = $2
ORDER BY date DESC
`, userID, guessID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var g GuessRecord
		if err := rows.Scan(&g.GuessId, &g.UserId, &g.GuessType, &g.Date, &g.Result); err != nil {
			return nil, err
		}
		grs = append(grs, &g)
	}
	return grs, err
}

func getAllGuessInTime(ctx context.Context) ([]*Guess, error) {
	guesses := make([]*Guess, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT client_id, asset_id, guess_id, symbol, price_usd, rules, explain, start_time, end_time, start_at, end_at, created_at 
FROM guess 
`)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var g Guess
		if err := rows.Scan(&g.ClientId, &g.AssetID, &g.GuessId, &g.Symbol, &g.PriceUsd, &g.Rules, &g.Explain, &g.StartTime, &g.EndTime, &g.StartAt, &g.EndAt, &g.CreatedAt); err != nil {
			return nil, err
		}
		guesses = append(guesses, &g)
	}
	return guesses, err
}

func UpdateGuessRecord(ctx context.Context) {
	gss, err := getAllGuessInTime(ctx)
	if err != nil {
		session.Logger(ctx).Println(err)
		return
	}
	for _, gs := range gss {
		yesterdayPrice, err := getGuessResult(ctx, gs.AssetID, "-1")
		if err != nil {
			session.Logger(ctx).Println(err)
			continue
		}
		todayPrice, err := getGuessResult(ctx, gs.AssetID, "")
		if err != nil {
			session.Logger(ctx).Println(err)
			continue
		}
		todayType := GuessTypeFlat
		if todayPrice.Cmp(yesterdayPrice) > 0 {
			todayType = GuessTypeUP
		} else if todayPrice.Cmp(yesterdayPrice) < 0 {
			todayType = GuessTypeDown
		}
		grs := make([]*GuessRecord, 0)
		rows, err := session.Database(ctx).Query(ctx, `
SELECT user_id, guess_type
FROM guess_record
WHERE guess_id=$1 AND date=current_date-1
`, func(rows pgx.Rows) error {
			return nil
		}, gs.GuessId)
		if err != nil {
			session.Logger(ctx).Println(err)
			continue
		}
		for rows.Next() {
			var gr GuessRecord
			if rows.Scan(&gr.UserId, &gr.GuessType); err != nil {
				return
			}
			grs = append(grs, &gr)
		}

		for _, gr := range grs {
			result := GuessResultPending
			if gr.GuessType == todayType {
				result = GuessResultWin
			} else {
				result = GuessResultLose
			}
			if _, err := session.Database(ctx).Exec(ctx, `
UPDATE guess_record
SET result=$1
WHERE user_id=$2 AND guess_id=$3 AND date=current_date-1
`, result, gr.UserId, gs.GuessId); err != nil {
				session.Logger(ctx).Println(err)
				continue
			}
		}
	}
}

func getGuessResult(ctx context.Context, assetID, offset string) (decimal.Decimal, error) {
	var r decimal.Decimal
	err := session.Database(ctx).QueryRow(ctx, fmt.Sprintf(`
SELECT price
FROM guess_result
WHERE asset_id = $1 AND date = current_date%s
`, offset), assetID).Scan(&r)
	return r, err
}

func GetAllGuessRecordByGuessID(ctx context.Context, guessID string) ([]*GuessRecord, error) {
	grs := make([]*GuessRecord, 0)
	rows, err := session.Database(ctx).Query(ctx, `
SELECT user_id,result,to_char(date, 'YYYY-MM-DD') AS date
FROM guess_record
WHERE guess_id = $1
ORDER BY date DESC
`, func(rows pgx.Rows) error {
		return nil
	}, guessID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var gr GuessRecord
		if err := rows.Scan(&gr.UserId, &gr.Result, &gr.Date); err != nil {
			return nil, err
		}
		grs = append(grs, &gr)
	}
	return grs, err
}

const (
	TRX_ID             = "tron"
	TRX_ASSET_ID       = "25dabac5-056a-48ff-b9f9-f67395dc407c"
	DEFAULT_CURRENCIES = "usd"
)

// func timeToUpdateGuessResult() {
// 	c := cron.New(cron.WithLocation(time.UTC))
// 	_, err := c.AddFunc("0 0 * * *", func() {
// 		insertQuery := durable.InsertQuery("guess_result", "asset_id,price")
// 		trxPrice := getSimplePrice()
// 		_, err := session.Database(_ctx).Exec(_ctx, insertQuery, "25dabac5-056a-48ff-b9f9-f67395dc407c", trxPrice)
// 		if err != nil {
// 			session.Logger(_ctx).Println(err)
// 		}
// 		_, err = session.Database(_ctx).Exec(_ctx, `
// UPDATE guess
// SET price_usd=$2
// WHERE asset_id=$1`, "25dabac5-056a-48ff-b9f9-f67395dc407c", trxPrice)
// 		if err != nil {
// 			session.Logger(_ctx).Println(err)
// 		}
// 		UpdateGuessRecord(_ctx)
// 	})
// 	if err != nil {
// 		session.Logger(_ctx).Println(err)
// 	}
// 	c.Start()
// }

// func getSimplePrice() decimal.Decimal {
// 	httpClient := &http.Client{
// 		Timeout: time.Second * 10,
// 	}
// 	CG := coingecko.NewClient(httpClient)
// 	trx, err := CG.SimplePrice([]string{TRX_ID}, []string{DEFAULT_CURRENCIES})
// 	if err != nil {
// 		session.Logger(_ctx).Println(err)
// 		return getSimplePrice()
// 	}
// 	trxPrice := (*trx)[TRX_ID][DEFAULT_CURRENCIES]
// 	return decimal.NewFromFloat32(trxPrice)
// }
