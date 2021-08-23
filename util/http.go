package util

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

func CopyReq(req *http.Request) *http.Request {
	r := *req
	buf, _ := ioutil.ReadAll(r.Body)
	req.Body = ioutil.NopCloser(bytes.NewBuffer(buf))
	r.Body = ioutil.NopCloser(bytes.NewBuffer(buf))
	return &r
}

func ResponseBody(obj interface{}) []byte {
	respBody, err := json.Marshal(obj)
	if err != nil {
		return []byte(`{\"errors\":[\"Failed to parse response body\"]}`)
	}
	return respBody
}

func ResponseOKWithBody(rw http.ResponseWriter, obj interface{}) {
	rw.Header().Set("Content-type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(ResponseBody(obj))
}

func ResponseOK(rw http.ResponseWriter) {
	rw.WriteHeader(http.StatusOK)
}

func ResponseError(rw http.ResponseWriter, statusCode int, err error) {
	ResponseErrorMsg(rw, statusCode, err.Error())
}

func ResponseErrorMsg(rw http.ResponseWriter, statusCode int, errMsg string) {
	rw.WriteHeader(statusCode)
	_, _ = rw.Write(ResponseBody([]string{errMsg}))
}
