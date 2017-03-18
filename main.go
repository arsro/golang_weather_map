package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

type (
	// 2つの外部APIを叩くためそれら２つの振る舞いを同じ振る舞いをしてほしいので、インターフェース化する
	weatherProvider interface {
		temperature(city string) (float64, error)
	}

	multiWeatherProvider []weatherProvider

	openWeatherMap struct {
		apiKey string
	}

	weatherUnderground struct {
		apiKey string
	}
)

func main() {

	mw := multiWeatherProvider{
		openWeatherMap{apiKey: "980d3ad176c472f80193c166ba5bf3e5"},
		weatherUnderground{apiKey: "333b06e95c6ea021"},
	}

	http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		city := strings.SplitN(r.URL.Path, "/", 3)[2]

		temp, err := mw.temperature(city)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"city": city,
			"temp": temp,
			"took": time.Since(begin).String(),
		})
	})

	http.ListenAndServe(":8080", nil)
}

func (mw multiWeatherProvider) temperature(city string) (float64, error) {

	// make channel
	temps := make(chan float64)
	errs := make(chan error)

	// goroutine
	// see http://gihyo.jp/dev/feature/01/go_4beginners/0005?page=3
	for _, provider := range mw {
		go func(p weatherProvider) {
			k, err := p.temperature(city)
			if err != nil {
				errs <- err
				return
			}
			temps <- k
		}(provider)
	}

	sum := 0.0
	// 5秒後に値が読み出せるチャネル
	// 指定した秒数によっては途中でタイムアウトするので、上で取得した2つのデータの合計値を取得することができない
	timeout := time.After(5 * time.Second)
	for i := 0; i < len(mw); i++ {
		select {
		case temp := <-temps:
			sum += temp
		case err := <-errs:
			return 0, err
		case <-timeout:
			break
		}
	}

	// Return the average, same as before.
	return sum / float64(len(mw)), nil
}

func (w openWeatherMap) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?q=" + city + "&appid=" + w.apiKey)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()
	time.Sleep(3 * time.Second)
	//response type
	var d struct {
		Main struct {
			Kelvin float64 `json:"temp"`
		} `json:"main"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	log.Printf("openWeatherMap: %s: %.2f", city, d.Main.Kelvin)
	return d.Main.Kelvin, nil
}

func (w weatherUnderground) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.wunderground.com/api/" + w.apiKey + "/conditions/q/" + city + ".json")
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	//response type
	var d struct {
		Observation struct {
			Celsius float64 `json:"temp_c"`
		} `json:"current_observation"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	kelvin := d.Observation.Celsius + 273.15
	log.Printf("weatherUnderground: %s: %.2f", city, kelvin)
	return kelvin, nil
}
