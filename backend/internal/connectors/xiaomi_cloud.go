package connectors

import (
	"context"
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Client for the undocumented Xiaomi Home "eco/scale" cloud API, which holds
// the body-composition history the Mi Home app shows. The regular MIoT
// property API cannot be used: every property of yunmai.scales.ms10x is
// declared notify-only, so /miotspec/prop/get returns nothing.
//
// Auth is passToken-based on purpose. The password flow answers with a captcha
// and an emailed 2FA code, which a scheduled sync cannot satisfy. Obtain the
// passToken once with PiotrMachowski/Xiaomi-cloud-tokens-extractor; the login
// response carries a replacement whenever Xiaomi decides to rotate it.
const (
	xiaomiLoginURL      = "https://account.xiaomi.com/pass/serviceLogin?sid=xiaomiio&_json=true"
	xiaomiLoginPrefix   = "&&&START&&&"
	xiaomiScalePath     = "/eco/common/scale/getUserDataByPage"
	xiaomiUserAgent     = "androidapp-ABCDEFGHIJKLM APP/com.xiaomi.mihome APPV/10.5.201"
	xiaomiPageSize      = 20
	xiaomiMaxPages      = 200
	xiaomiRequestTimout = 30 * time.Second
)

// XiaomiScaleRecord is one weigh-in. Data holds the metrics as a JSON string.
type XiaomiScaleRecord struct {
	Data       string `json:"data"`
	CreateTime int64  `json:"createTime"`
	Model      string `json:"model"`
	Did        string `json:"did"`
	SN         string `json:"sn"`
	UID        int64  `json:"uid"`
	AccountID  int64  `json:"accountId"`
	FromSource int    `json:"fromSource"`
}

// MeasuredAt is the authoritative timestamp. The inner "data" object also
// carries a "time" field, but its unit is inconsistent across record types.
func (r XiaomiScaleRecord) MeasuredAt() time.Time {
	return time.UnixMilli(r.CreateTime)
}

type XiaomiCloud struct {
	http         *http.Client
	region       string
	accountID    string
	ssecurity    []byte
	serviceToken string
	passToken    string
}

func NewXiaomiCloud(region string) *XiaomiCloud {
	if region == "" {
		region = "ru"
	}
	jar, _ := cookiejar.New(nil)
	return &XiaomiCloud{
		http:   &http.Client{Timeout: xiaomiRequestTimout, Jar: jar},
		region: region,
	}
}

// PassToken returns the (possibly refreshed) token to persist after a sync.
func (x *XiaomiCloud) PassToken() string { return x.passToken }

// Login exchanges a stored passToken for the ssecurity secret and a
// serviceToken. accountID is the numeric Xiaomi user id, which the endpoint
// requires - passToken alone is rejected with code 70016.
func (x *XiaomiCloud) Login(ctx context.Context, accountID, passToken string) error {
	x.accountID = accountID

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, xiaomiLoginURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", xiaomiUserAgent)
	req.Header.Set("Cookie", "sdkVersion=accountsdk-18.8.15; userId="+accountID+"; passToken="+passToken)

	res, err := x.http.Do(req)
	if err != nil {
		return err
	}
	body, err := readXiaomiLoginBody(res)
	if err != nil {
		return err
	}

	var login struct {
		Ssecurity string `json:"ssecurity"`
		PassToken string `json:"passToken"`
		UserID    int64  `json:"userId"`
		Location  string `json:"location"`
		Code      int    `json:"code"`
		Desc      string `json:"desc"`
	}
	if err := json.Unmarshal(body, &login); err != nil {
		return fmt.Errorf("decode login: %w", err)
	}
	if login.Ssecurity == "" {
		return fmt.Errorf("passToken rejected (code=%d %s) — re-extract it in Settings", login.Code, login.Desc)
	}

	x.ssecurity, err = base64.StdEncoding.DecodeString(login.Ssecurity)
	if err != nil {
		return fmt.Errorf("decode ssecurity: %w", err)
	}
	x.passToken = firstNonEmpty(login.PassToken, passToken)
	if login.UserID != 0 {
		x.accountID = fmt.Sprint(login.UserID)
	}

	// Following the location redirect is what sets the serviceToken cookie.
	stsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, login.Location, nil)
	if err != nil {
		return err
	}
	stsReq.Header.Set("User-Agent", xiaomiUserAgent)
	stsRes, err := x.http.Do(stsReq)
	if err != nil {
		return fmt.Errorf("sts: %w", err)
	}
	io.Copy(io.Discard, stsRes.Body)
	stsRes.Body.Close()

	stsURL, err := url.Parse(login.Location)
	if err != nil {
		return err
	}
	for _, cookie := range x.http.Jar.Cookies(stsURL) {
		if cookie.Name == "serviceToken" {
			x.serviceToken = cookie.Value
		}
	}
	if x.serviceToken == "" {
		return fmt.Errorf("no serviceToken in sts response")
	}
	return nil
}

// FetchSince walks the history backwards in pages of xiaomiPageSize until it
// reaches a record older than cutoff. A zero cutoff pulls everything.
func (x *XiaomiCloud) FetchSince(ctx context.Context, model string, cutoff time.Time) ([]XiaomiScaleRecord, error) {
	var all []XiaomiScaleRecord
	begin := time.Now().UnixMilli()

	for page := range xiaomiMaxPages {
		payload := fmt.Sprintf(
			`{"endTime":1,"beginTime":%d,"model":"%s","uid":"%s","did":0,"accountId":0}`,
			begin, model, x.accountID)

		raw, err := x.request(ctx, xiaomiScalePath, payload, model)
		if err != nil {
			return all, fmt.Errorf("page %d: %w", page, err)
		}

		var items []XiaomiScaleRecord
		if err := json.Unmarshal(raw, &items); err != nil {
			return all, fmt.Errorf("page %d: decode: %w", page, err)
		}
		if len(items) == 0 {
			break
		}
		all = append(all, items...)

		oldest := items[len(items)-1].CreateTime
		if len(items) < xiaomiPageSize || oldest == 0 || oldest >= begin {
			break
		}
		if !cutoff.IsZero() && oldest <= cutoff.UnixMilli() {
			break
		}
		begin = oldest
	}
	return all, nil
}

// request performs one signed, RC4-encrypted miio call. Fields travel in the
// query string, and the signature covers the API path WITHOUT the /app prefix.
func (x *XiaomiCloud) request(ctx context.Context, apiPath, payload, model string) (json.RawMessage, error) {
	base := "https://" + x.region + ".api.io.mi.com/app"
	if x.region == "cn" {
		base = "https://api.io.mi.com/app"
	}

	nonce, err := xiaomiNonce()
	if err != nil {
		return nil, err
	}
	signedNonce := xiaomiSignedNonce(x.ssecurity, nonce)
	signedNonce64 := base64.StdEncoding.EncodeToString(signedNonce)

	// Sign the plaintext, encrypt it, then sign the ciphertext. Order matters.
	fields := []string{"data", payload}
	fields = append(fields, "rc4_hash__", xiaomiSign(apiPath, fields, signedNonce64))
	for i := 1; i < len(fields); i += 2 {
		encrypted, err := xiaomiRC4(signedNonce, []byte(fields[i]))
		if err != nil {
			return nil, err
		}
		fields[i] = base64.StdEncoding.EncodeToString(encrypted)
	}

	query := url.Values{}
	for i := 0; i < len(fields); i += 2 {
		query.Set(fields[i], fields[i+1])
	}
	query.Set("signature", xiaomiSign(apiPath, fields, signedNonce64))
	query.Set("ssecurity", base64.StdEncoding.EncodeToString(x.ssecurity))
	query.Set("_nonce", base64.StdEncoding.EncodeToString(nonce))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+apiPath+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", xiaomiUserAgent)
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-xiaomi-protocal-flag-cli", "PROTOCAL-HTTP2")
	req.Header.Set("MIOT-ENCRYPT-ALGORITHM", "ENCRYPT-RC4")
	req.Header.Set("MIOT-REQUEST-MODEL", model)
	req.Header.Set("Cookie", strings.Join([]string{
		"userId=" + x.accountID,
		"serviceToken=" + x.serviceToken,
		"yetAnotherServiceToken=" + x.serviceToken,
		"locale=en_GB",
		"timezone=GMT+03:00",
		"is_daylight=1",
		"dst_offset=0",
		"channel=MI_APP_STORE",
	}, "; "))

	res, err := x.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %.200s", res.StatusCode, body)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return nil, fmt.Errorf("response not base64: %.200s", body)
	}
	plaintext, err := xiaomiRC4(signedNonce, ciphertext)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(plaintext, &resp); err != nil {
		return nil, fmt.Errorf("decode: %w (%.200s)", err, plaintext)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("api %d: %s", resp.Code, resp.Message)
	}
	return resp.Result, nil
}

func readXiaomiLoginBody(res *http.Response) ([]byte, error) {
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if !strings.HasPrefix(string(body), xiaomiLoginPrefix) {
		return nil, fmt.Errorf("unexpected login response: %.100s", body)
	}
	return body[len(xiaomiLoginPrefix):], nil
}

func xiaomiNonce() ([]byte, error) {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce[:8]); err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint32(nonce[8:], uint32(time.Now().UnixMilli()/60000))
	return nonce, nil
}

func xiaomiSignedNonce(ssecurity, nonce []byte) []byte {
	h := sha256.New()
	h.Write(ssecurity)
	h.Write(nonce)
	return h.Sum(nil)
}

func xiaomiRC4(key, data []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	cipher.XORKeyStream(make([]byte, 1024), make([]byte, 1024))
	out := make([]byte, len(data))
	cipher.XORKeyStream(out, data)
	return out, nil
}

// xiaomiSign builds SHA1("POST&<path>&k=v&...&<signedNonce>") over flat pairs.
// The path must be supplied explicitly: deriving it by splitting the URL on
// "com" (as the reference Python client does) truncates any path containing
// that substring, and ours contains "common".
func xiaomiSign(apiPath string, fields []string, signedNonce64 string) string {
	parts := make([]string, 0, len(fields)/2+3)
	parts = append(parts, http.MethodPost, apiPath)
	for i := 0; i < len(fields); i += 2 {
		parts = append(parts, fields[i]+"="+fields[i+1])
	}
	parts = append(parts, signedNonce64)

	h := sha1.New()
	h.Write([]byte(strings.Join(parts, "&")))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
