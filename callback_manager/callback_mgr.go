package callback_mgr

import (
	"fmt"
	"reflect"
)

var callBackMap map[string]interface{}

func init() {
	callBackMap = make(map[string]interface{})
}

func RegisterCallBack(key string, callBack interface{}) {
	callBackMap[key] = callBack
}

func IsExistCallBack(key string) bool {
	_, ok := callBackMap[key]
	if ok {
		return true
	} else {
		return false
	}
}

func CallBackFunc(key string, args ...interface{}) []interface{} {
	if callBack, ok := callBackMap[key]; ok {
		in := make([]reflect.Value, len(args))
		for i, arg := range args {
			in[i] = reflect.ValueOf(arg)
		}
		outList := reflect.ValueOf(callBack).Call(in)
		result := make([]interface{}, len(outList))
		for i, out := range outList {
			result[i] = out.Interface()
		}
		return result
	} else {
		panic(fmt.Errorf("callBack(%s) not found", key))
	}
}
