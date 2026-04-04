package main

import (
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const loginPrefix = "&&&START&&&"

type client struct {
	http      *http.Client
	cookies   string
	ssecurity []byte
	userID    int64
}

func main() {
	username := os.Getenv("XIAOMI_USER")
	password := os.Getenv("XIAOMI_PASS")
	region := os.Getenv("XIAOMI_REGION")
	deviceID := os.Getenv("XIAOMI_DEVICE_ID")
	if region == "" {
		region = "ru"
	}
	if username == "" || password == "" {
		log.Fatal("set XIAOMI_USER and XIAOMI_PASS")
	}
	if deviceID == "" {
		log.Fatal("set XIAOMI_DEVICE_ID (from browser cookie)")
	}

	c := &client{http: &http.Client{Timeout: time.Minute}}

	sid := "xiaomiio"
	log.Printf("login: user=%s sid=%s deviceId=%s", username, sid, deviceID)

	// step 1
	res, err := c.http.Get("https://account.xiaomi.com/pass/serviceLogin?_json=true&sid=" + sid)
	if err != nil {
		log.Fatal("step1: ", err)
	}
	body1, err := readBody(res)
	if err != nil {
		log.Fatal("step1 body: ", err)
	}

	var s1 struct {
		Qs       string `json:"qs"`
		Sign     string `json:"_sign"`
		Sid      string `json:"sid"`
		Callback string `json:"callback"`
	}
	json.Unmarshal(body1, &s1)
	log.Println("step1 ok")

	// step 2 — use browser's deviceId
	hash := fmt.Sprintf("%X", md5.Sum([]byte(password)))
	form := url.Values{
		"_json":    {"true"},
		"hash":     {hash},
		"sid":      {s1.Sid},
		"callback": {s1.Callback},
		"_sign":    {s1.Sign},
		"qs":       {s1.Qs},
		"user":     {username},
	}

	req, _ := http.NewRequest("POST", "https://account.xiaomi.com/pass/serviceLoginAuth2", strings.NewReader(form.Encode()))
	req.Header.Set("Cookie", "deviceId="+deviceID)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err = c.http.Do(req)
	if err != nil {
		log.Fatal("step2: ", err)
	}
	body2, err := readBody(res)
	if err != nil {
		log.Fatal("step2 body: ", err)
	}

	var s2 struct {
		Ssecurity      json.RawMessage `json:"ssecurity"`
		PassToken      string          `json:"passToken"`
		UserId         int64           `json:"userId"`
		Location       string          `json:"location"`
		SecurityStatus int             `json:"securityStatus"`
		Desc           string          `json:"desc"`
	}
	json.Unmarshal(body2, &s2)

	log.Printf("step2: securityStatus=%d desc=%s location_len=%d", s2.SecurityStatus, s2.Desc, len(s2.Location))

	if s2.Location == "" {
		log.Fatal("step2 failed: still requires verification. response: ", string(body2))
	}

	// decode ssecurity (can be string or base64)
	var ssecStr string
	json.Unmarshal(s2.Ssecurity, &ssecStr)
	ssec, err := base64.StdEncoding.DecodeString(ssecStr)
	if err != nil {
		ssec = []byte(ssecStr)
	}
	c.ssecurity = ssec
	c.userID = s2.UserId
	log.Printf("step2 ok: userId=%d ssecurity_len=%d", c.userID, len(c.ssecurity))

	// step 3 — follow location
	res, err = c.http.Get(s2.Location)
	if err != nil {
		log.Fatal("step3: ", err)
	}
	io.ReadAll(res.Body)
	res.Body.Close()
	for _, s := range res.Header["Set-Cookie"] {
		s, _, _ = strings.Cut(s, ";")
		if len(c.cookies) > 0 {
			c.cookies += "; "
		}
		c.cookies += s
	}
	log.Printf("step3 ok: %d cookie parts", strings.Count(c.cookies, ";")+1)

	// fetch scale data
	models := []string{"MJTZC01YM"}
	for _, model := range models {
		log.Printf("=== scale API model=%s region=%s ===", model, region)

		var baseURL, apiPath, params string
		if region == "" || region == "cn" {
			baseURL = "https://api.io.mi.com/app"
			apiPath = "/eco/scale/getData"
			params = fmt.Sprintf(`{"param":{"endTime":1,"beginTime":%d},"model":"%s","uid":%d,"did":0}`, time.Now().UnixMilli(), model, c.userID)
		} else {
			baseURL = "https://" + region + ".api.io.mi.com/app"
			apiPath = "/eco/common/scale/getUserDataByPage"
			params = fmt.Sprintf(`{"endTime":1,"beginTime":%d,"model":"%s","uid":"%d","did":0,"accountId":0}`, time.Now().UnixMilli(), model, c.userID)
		}

		data, err := c.request(baseURL, apiPath, params, map[string]string{
			"MIOT-REQUEST-MODEL": model,
		})
		if err != nil {
			log.Printf("  error: %v", err)
			continue
		}

		log.Printf("  raw result: %s", string(data[:min(2000, len(data))]))
	}

	// also try Mi Fitness health API
	log.Println("=== Mi Fitness health API ===")
	ts := time.Now().Add(24 * time.Hour).Unix()
	healthParams := fmt.Sprintf(`{"start_time":1,"end_time":%d,"key":"weight"}`, ts)
	baseURL := "https://" + region + ".hlth.io.mi.com"

	data, err := c.request(baseURL, "/app/v1/data/get_fitness_data_by_time", healthParams, nil)
	if err != nil {
		log.Println("  error:", err)
	} else {
		log.Printf("  result: %s", string(data[:min(2000, len(data))]))
	}
}

func (c *client) request(baseURL, apiURL, params string, headers map[string]string) ([]byte, error) {
	form := url.Values{"data": {params}}
	nonce := genNonce()
	signedNonce := genSignedNonce(c.ssecurity, nonce)

	form.Set("rc4_hash__", genSignature64("POST", apiURL, form, signedNonce))
	for _, v := range form {
		ct, _ := crypt(signedNonce, []byte(v[0]))
		v[0] = base64.StdEncoding.EncodeToString(ct)
	}
	form.Set("signature", genSignature64("POST", apiURL, form, signedNonce))
	form.Set("_nonce", base64.StdEncoding.EncodeToString(nonce))

	req, _ := http.NewRequest("POST", baseURL+apiURL, strings.NewReader(form.Encode()))
	req.Header.Set("Cookie", c.cookies)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", res.StatusCode, string(body[:min(200, len(body))]))
	}

	ct, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return body, nil
	}

	pt, _ := crypt(signedNonce, ct)
	var resp struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(pt, &resp); err != nil {
		return nil, fmt.Errorf("json: %w (%.200s)", err, string(pt))
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("api %d: %s", resp.Code, resp.Message)
	}
	return resp.Result, nil
}

func readBody(res *http.Response) ([]byte, error) {
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if len(body) < len(loginPrefix) || string(body[:len(loginPrefix)]) != loginPrefix {
		return nil, fmt.Errorf("wrong prefix: %.100s", body)
	}
	return body[len(loginPrefix):], nil
}

func genNonce() []byte {
	ts := time.Now().Unix() / 60
	nonce := make([]byte, 12)
	for i := range nonce[:8] {
		nonce[i] = byte(rand.Intn(256))
	}
	binary.BigEndian.PutUint32(nonce[8:], uint32(ts))
	return nonce
}

func genSignedNonce(ssecurity, nonce []byte) []byte {
	h := sha256.New()
	h.Write(ssecurity)
	h.Write(nonce)
	return h.Sum(nil)
}

func crypt(key, data []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	tmp := make([]byte, 1024)
	cipher.XORKeyStream(tmp, tmp)
	out := make([]byte, len(data))
	cipher.XORKeyStream(out, data)
	return out, nil
}

func genSignature64(method, path string, values url.Values, signedNonce []byte) string {
	s := method + "&" + path + "&data=" + values.Get("data")
	if values.Has("rc4_hash__") {
		s += "&rc4_hash__=" + values.Get("rc4_hash__")
	}
	s += "&" + base64.StdEncoding.EncodeToString(signedNonce)
	h := sha1.New()
	h.Write([]byte(s))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
