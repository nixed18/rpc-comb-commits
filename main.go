package main

import (
	//"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	//"flag"

	//"log"
	//"os/exec"
)

var username = "user"
var password = "password"

type UserConfig struct {
	username string
	password string
	regtest bool
}

func make_client() *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
		},
		Timeout: 300 * time.Second,
	}
	return client
}

func make_haircomb_call(url string, wait bool) string {
	resp, err := http.Get("http://127.0.0.1:2121"+url)
	if err != nil {
		fmt.Println("phone comb ERROR", err)
	}

	defer resp.Body.Close()

	if wait {
		resp_bytes, err :=  ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println(fmt.Println("phone comb ERROR 2", err))

		}
		return string(resp_bytes)
	}
	return "doesntmatter"
	
} 

func make_bitcoin_call(client *http.Client, method, params string, u_config *UserConfig) interface{} {

	fmt.Println("MAKE CALL", method, params)
	// make post
	body := strings.NewReader("{\"jsonrpc\":\"1.0\",\"id\":\"curltext\",\"method\":\""+method+"\",\"params\":["+params+"]}")
	req, err := http.NewRequest("POST", "http://"+u_config.username+":"+u_config.password+"@127.0.0.1:8332", body)


	if err != nil {
		fmt.Println("phone btc ERROR", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(fmt.Println("phone btc ERROR 2", err))

	}

	defer resp.Body.Close()
	fmt.Println("DO DONE")
	resp_bytes, err :=  ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(fmt.Println("phone btc ERROR 3", err))

	}
	var result map[string]interface{}

	err = json.Unmarshal(resp_bytes, &result)
	if err != nil {
		fmt.Println("ERROR", err)
	}

	return result["result"]

}



func reorg_check(client *http.Client, hc_height int, u_config *UserConfig) bool {
	// Reference the mined blocks db for the most recent block hash, and compare it against the hash of the block in the BTC chain
	if get_hc_block_hash(hc_height) == strings.TrimRight(fmt.Sprintf("%v", make_bitcoin_call(client, "getblockhash", fmt.Sprint(hc_height), u_config)), "\r\n") {
		return false
	} 
	return true
}

func get_hc_block_hash(height int) string {
	// Pull from the stored DB of HC's mined blocks
	return "hash_here"
}

func main() {
	// Define the user config
	u_config := &UserConfig{
		username: username,
		password: password,
		regtest: false,

	}

	// make the http client
	http_client := make_client()

	// ping haircomb for highest known block
	base_height := make_haircomb_call("/height/get", true)
	fmt.Println(base_height)
	// format curr_height
	curr_height, err := strconv.Atoi(base_height)
	if err != nil {
		log.Fatal(err)
		fmt.Println("stringtoint ERROR", err)
	}
	// move currheight to first comb block (481824)
	if curr_height < 481824 {
		curr_height = 481824
	}

	for {
		// wait 5 seconds
		time.Sleep(5 * time.Second)

		// Check for reorg

		// Pull the current BTC height
		btc_height := int(make_bitcoin_call(http_client, "getblockcount", "", u_config).(float64))
		// If caught up, skip this cycle
		if btc_height == curr_height {
			continue
		}

		curr_height = mine(MiningConfig{username: u_config.username, password: u_config.password, start_height: curr_height, target_height: btc_height, direction: 1, regtest: u_config.regtest})
	}
}

