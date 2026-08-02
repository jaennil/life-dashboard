// Command xiaomi-test dumps Xiaomi Body Composition Scale history from the
// Xiaomi Home cloud. Auth is passToken-based so it runs unattended: the
// password flow needs a captcha and an emailed 2FA code, which a cron job
// cannot answer.
//
// Obtain XIAOMI_USER_ID / XIAOMI_PASS_TOKEN / XIAOMI_DEVICE_ID once with
// PiotrMachowski/Xiaomi-cloud-tokens-extractor (answer the 2FA with
// trust=true and keep the same deviceId afterwards).
package main

import (
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	loginPrefix = "&&&START&&&"
	scalePath   = "/eco/common/scale/getUserDataByPage"
	pageSize    = 20
)

type client struct {
	http         *http.Client
	agent        string
	deviceID     string
	ssecurity    []byte
	userID       string
	serviceToken string
}

// scaleRecord is one weigh-in as returned by the cloud. Data is a JSON string.
type scaleRecord struct {
	Data       string `json:"data"`
	CreateTime int64  `json:"createTime"`
	Model      string `json:"model"`
	Did        string `json:"did"`
	SN         string `json:"sn"`
	FromSource int    `json:"fromSource"`
}

func main() {
	region := envOr("XIAOMI_REGION", "ru")
	model := envOr("XIAOMI_MODEL", "yunmai.scales.ms104")
	userID := os.Getenv("XIAOMI_USER_ID")
	passToken := os.Getenv("XIAOMI_PASS_TOKEN")
	deviceID := os.Getenv("XIAOMI_DEVICE_ID")
	outPath := os.Getenv("XIAOMI_OUT")

	if userID == "" || passToken == "" || deviceID == "" {
		log.Fatal("set XIAOMI_USER_ID, XIAOMI_PASS_TOKEN and XIAOMI_DEVICE_ID")
	}

	jar, _ := cookiejar.New(nil)
	c := &client{
		http:     &http.Client{Timeout: time.Minute, Jar: jar},
		agent:    "androidapp-ABCDEFGHIJKLM APP/com.xiaomi.mihome APPV/10.5.201",
		deviceID: deviceID,
		userID:   userID,
	}

	if err := c.login(passToken); err != nil {
		log.Fatal("login: ", err)
	}
	log.Printf("logged in: userId=%s ssecurity=%dB serviceToken=%dB",
		c.userID, len(c.ssecurity), len(c.serviceToken))

	records, err := c.fetchAll(region, model)
	if err != nil {
		log.Fatal("fetch: ", err)
	}
	log.Printf("fetched %d records", len(records))

	if len(records) > 0 {
		newest := records[0]
		log.Printf("newest: %s  %s",
			time.UnixMilli(newest.CreateTime).Format(time.RFC3339), newest.Data)
	}

	if outPath != "" {
		blob, _ := json.MarshalIndent(records, "", " ")
		if err := os.WriteFile(outPath, blob, 0o600); err != nil {
			log.Fatal("write: ", err)
		}
		log.Printf("wrote %s", outPath)
	}
}

// login exchanges a stored passToken for ssecurity + serviceToken. No password,
// no captcha, no 2FA.
func (c *client) login(passToken string) error {
	const loginURL = "https://account.xiaomi.com/pass/serviceLogin?sid=xiaomiio&_json=true"

	req, _ := http.NewRequest("GET", loginURL, nil)
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("Cookie", strings.Join([]string{
		"sdkVersion=accountsdk-18.8.15",
		"deviceId=" + c.deviceID,
		"userId=" + c.userID,
		"passToken=" + passToken,
	}, "; "))

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	body, err := readBody(res)
	if err != nil {
		return err
	}

	var login struct {
		Ssecurity string `json:"ssecurity"`
		UserID    int64  `json:"userId"`
		Location  string `json:"location"`
		Code      int    `json:"code"`
		Desc      string `json:"desc"`
	}
	if err := json.Unmarshal(body, &login); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if login.Ssecurity == "" {
		return fmt.Errorf("passToken rejected (code=%d desc=%s) - re-run the token extractor",
			login.Code, login.Desc)
	}

	c.ssecurity, err = base64.StdEncoding.DecodeString(login.Ssecurity)
	if err != nil {
		return fmt.Errorf("ssecurity: %w", err)
	}
	if login.UserID != 0 {
		c.userID = fmt.Sprint(login.UserID)
	}

	// Following location sets the serviceToken cookie on the STS domain.
	res, err = c.http.Get(login.Location)
	if err != nil {
		return fmt.Errorf("sts: %w", err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()

	stsURL, _ := url.Parse(login.Location)
	for _, ck := range c.http.Jar.Cookies(stsURL) {
		if ck.Name == "serviceToken" {
			c.serviceToken = ck.Value
		}
	}
	if c.serviceToken == "" {
		return fmt.Errorf("no serviceToken in STS response")
	}
	return nil
}

// fetchAll walks the history backwards, pageSize records at a time.
func (c *client) fetchAll(region, model string) ([]scaleRecord, error) {
	var all []scaleRecord
	begin := time.Now().UnixMilli()

	for page := range 200 {
		payload := fmt.Sprintf(
			`{"endTime":1,"beginTime":%d,"model":"%s","uid":"%s","did":0,"accountId":0}`,
			begin, model, c.userID)

		raw, err := c.request(region, scalePath, payload, model)
		if err != nil {
			return all, fmt.Errorf("page %d: %w", page, err)
		}

		var items []scaleRecord
		if err := json.Unmarshal(raw, &items); err != nil {
			return all, fmt.Errorf("page %d decode: %w (%.200s)", page, err, raw)
		}
		log.Printf("  page %d: %d records", page, len(items))
		all = append(all, items...)
		if len(items) < pageSize {
			break
		}

		next := items[len(items)-1].CreateTime
		if next == 0 || next >= begin {
			break
		}
		begin = next
	}
	return all, nil
}

// request performs one signed, RC4-encrypted miio call. Fields travel in the
// query string; the signature covers the API path WITHOUT the /app prefix.
func (c *client) request(region, apiPath, payload, model string) (json.RawMessage, error) {
	base := "https://" + region + ".api.io.mi.com/app"
	if region == "cn" {
		base = "https://api.io.mi.com/app"
	}

	nonce, err := genNonce()
	if err != nil {
		return nil, err
	}
	signedNonce := genSignedNonce(c.ssecurity, nonce)
	signedNonce64 := base64.StdEncoding.EncodeToString(signedNonce)

	// Sign plaintext, encrypt, then sign the ciphertext. Field order matters.
	fields := []string{"data", payload}
	fields = append(fields, "rc4_hash__", sign(apiPath, fields, signedNonce64))
	for i := 1; i < len(fields); i += 2 {
		ct, err := crypt(signedNonce, []byte(fields[i]))
		if err != nil {
			return nil, err
		}
		fields[i] = base64.StdEncoding.EncodeToString(ct)
	}

	query := url.Values{}
	for i := 0; i < len(fields); i += 2 {
		query.Set(fields[i], fields[i+1])
	}
	query.Set("signature", sign(apiPath, fields, signedNonce64))
	query.Set("ssecurity", base64.StdEncoding.EncodeToString(c.ssecurity))
	query.Set("_nonce", base64.StdEncoding.EncodeToString(nonce))

	req, _ := http.NewRequest("POST", base+apiPath+"?"+query.Encode(), nil)
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-xiaomi-protocal-flag-cli", "PROTOCAL-HTTP2")
	req.Header.Set("MIOT-ENCRYPT-ALGORITHM", "ENCRYPT-RC4")
	req.Header.Set("MIOT-REQUEST-MODEL", model)
	req.Header.Set("Cookie", strings.Join([]string{
		"userId=" + c.userID,
		"serviceToken=" + c.serviceToken,
		"yetAnotherServiceToken=" + c.serviceToken,
		"locale=en_GB",
		"timezone=GMT+03:00",
		"is_daylight=1",
		"dst_offset=0",
		"channel=MI_APP_STORE",
	}, "; "))

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %.200s", res.StatusCode, body)
	}

	ct, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return nil, fmt.Errorf("response not base64: %.200s", body)
	}
	pt, err := crypt(signedNonce, ct)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(pt, &resp); err != nil {
		return nil, fmt.Errorf("decode: %w (%.200s)", err, pt)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("api %d: %s", resp.Code, resp.Message)
	}
	return resp.Result, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func readBody(res *http.Response) ([]byte, error) {
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if !strings.HasPrefix(string(body), loginPrefix) {
		return nil, fmt.Errorf("wrong prefix: %.100s", body)
	}
	return body[len(loginPrefix):], nil
}

func genNonce() ([]byte, error) {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce[:8]); err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint32(nonce[8:], uint32(time.Now().UnixMilli()/60000))
	return nonce, nil
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
	cipher.XORKeyStream(make([]byte, 1024), make([]byte, 1024))
	out := make([]byte, len(data))
	cipher.XORKeyStream(out, data)
	return out, nil
}

// sign builds SHA1("POST&<path>&k=v&...&<signedNonce>") over flat key/value pairs.
func sign(apiPath string, fields []string, signedNonce64 string) string {
	parts := make([]string, 0, len(fields)/2+3)
	parts = append(parts, "POST", apiPath)
	for i := 0; i < len(fields); i += 2 {
		parts = append(parts, fields[i]+"="+fields[i+1])
	}
	parts = append(parts, signedNonce64)

	h := sha1.New()
	h.Write([]byte(strings.Join(parts, "&")))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
