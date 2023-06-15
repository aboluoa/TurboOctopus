package model

import (
	"database/sql/driver"
	"fmt"
	"time"
)

const (
	YYYYMMDDHHMISS = "2006-01-02 15:04:05" //常规类型
)

type JSONTime struct {
	time.Time
}

// MarshalJSON on JSONTime format Time field with %Y-%m-%d %H:%M:%S
func (t JSONTime) MarshalJSON() ([]byte, error) {
	//formatted := fmt.Sprintf("\"%s\"", t.Format(YYYYMMDDHHMISS))
	formatted := fmt.Sprintf(`"%s"`, t.Time.Format(YYYYMMDDHHMISS))
	return []byte(formatted), nil
}

//func (t JSONTime) MarshalJSON() ([]byte, error) {
//	b := make([]byte, 0)
//	b = t.Time.AppendFormat(b, YYYYMMDDHHMISS)
//	return b, nil
//}

// Value insert timestamp into mysql need this function.
func (t JSONTime) Value() (driver.Value, error) {
	var zeroTime time.Time
	if t.Time.UnixNano() == zeroTime.UnixNano() {
		return nil, nil
	}
	return t.Time, nil
}

// Scan valueof time.Time
func (t *JSONTime) Scan(v interface{}) error {
	value, ok := v.(time.Time)
	if ok {
		*t = JSONTime{Time: value}
		return nil
	} else {
		bs, ok := v.([]byte)
		if ok {
			ts, err := time.ParseInLocation(YYYYMMDDHHMISS, string(bs), time.Local)
			t.Time = ts
			return err
		}
	}
	return fmt.Errorf("can not convert %v to timestamp", v)
}
func (t *JSONTime) UnmarshalJSON(data []byte) error {
	// Ignore null, like in the main JSON package.
	if string(data) == "null" {
		return nil
	}
	// Fractional seconds are handled implicitly by Parse.
	var err error
	(*t).Time, err = time.ParseInLocation(`"`+YYYYMMDDHHMISS+`"`, string(data), time.Local)
	return err
}

func (t *JSONTime) String() string {
	str := t.Time.String()
	str = str[:19]
	return str
}

type JSONStartTime struct {
	time.Time
}
