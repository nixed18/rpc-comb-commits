package main

import (
	//"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	//"flag"
	"github.com/syndtr/goleveldb/leveldb"
	//"log"
	//"os/exec"
)

var username = "user"
var password = "password"

type UserConfig struct {
	username string
	password string
	regtest bool
	mined_db_path string
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

	port := "8332"
	if u_config.regtest{
		port = "18443"
	}

	fmt.Println("MAKE CALL", method, params)
	// make post
	body := strings.NewReader("{\"jsonrpc\":\"1.0\",\"id\":\"curltext\",\"method\":\""+method+"\",\"params\":["+params+"]}")
	req, err := http.NewRequest("POST", "http://"+u_config.username+":"+u_config.password+"@127.0.0.1:"+port, body)


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
	// Load the DB
	db, err := leveldb.OpenFile(u_config.mined_db_path, nil)
	if err != nil {
		log.Fatal("DB open error", err)
	}
	defer db.Close()

	// Reference the mined blocks db for the most recent block hash, and compare it against the hash of the block in the BTC chain
	hc_hash, err := db.Get([]byte(fmt.Sprint(hc_height)), nil)
	if err != nil {
		// If key can't be found, return true
		if err.Error() == "leveldb: not found" {
			fmt.Println("p3")
			return true
		}
		log.Fatal("DB GET ERROR", err)
	}

	btc_hash := strings.TrimRight(fmt.Sprintf("%v", make_bitcoin_call(client, "getblockhash", fmt.Sprint(hc_height), u_config)), "\r\n")

	if string(hc_hash) == btc_hash {
		fmt.Println("NO REORG AT HEIGHT", fmt.Sprint(hc_height), ":", string(hc_hash), btc_hash)
		return false
	} 
	fmt.Println("REORG AT HEIGHT", fmt.Sprint(hc_height), ":", string(hc_hash), btc_hash)
	return true
}


func find_reorg(client *http.Client, hc_height int, u_config *UserConfig) int {

	// Set an stop limit
	lowest := 481824
	if u_config.regtest {
		lowest = 0
	}

	// Load the DB
	db, err := leveldb.OpenFile(u_config.mined_db_path, nil)
	if err != nil {
		log.Fatal("DB open error", err)
	}
	defer db.Close()

	// Compare until found
	for hc_height > lowest {
		hc_hash, err := db.Get([]byte(fmt.Sprint(hc_height)), nil)
		if err != nil {
			if err.Error() == "leveldb: not found" {
				fmt.Println("p4")
				hc_height--
				continue
			}
			log.Fatal("DB GET ERROR", err)
		}
		if string(hc_hash) == strings.TrimRight(fmt.Sprintf("%v", make_bitcoin_call(client, "getblockhash", fmt.Sprint(hc_height), u_config)), "\r\n") {
			return hc_height
		}
		hc_height--
	}
	return hc_height
}


func main() {
	// Setup Logging
	logfile, err := os.OpenFile("info.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer logfile.Close()
	log.SetOutput(logfile)

	// Define the user config
	u_config := &UserConfig{
		username: username,
		password: password,
		regtest: true,
		mined_db_path: "mined_blocks",
	}

	// make the http client for main calls
	http_client := make_client()

	// ping haircomb for highest known block
	base_height := make_haircomb_call("/height/get", true)
	fmt.Println(base_height)

	// format curr_height
	curr_height, err := strconv.Atoi(base_height)
	if err != nil {
		log.Fatal("stringtoint ERROR", err)
	}

	// If not regtest, make the current height the first COMB block
	if !u_config.regtest && curr_height < 481824 {
		curr_height = 481824
	}

	for {
		// wait 5 seconds
		time.Sleep(5 * time.Second)

		// Set the initial mine_dir
		mine_dir := 1

		// Pull the current BTC height
		target_height := int(make_bitcoin_call(http_client, "getblockcount", "", u_config).(float64))
		
		// If caught up, skip this cycle
		if target_height == curr_height {
			continue
		}

		fmt.Println("CURR: ", curr_height, ", TARGET: ", target_height)

		// Set reorg check lowest
		lowest := 481824
		if u_config.regtest {
			lowest = 0
		}

		// Check for reorg
		if curr_height > lowest && reorg_check(http_client, curr_height, u_config) {
			// If reorg, then identify the block height to reorg to
			target_height = find_reorg(http_client, curr_height, u_config)
			fmt.Println(2)
			// Set the mine_dir to -1
			mine_dir = -1
			fmt.Println("REORG: ", curr_height, " ", target_height)
		}



		curr_height = mine(MiningConfig{username: u_config.username, password: u_config.password, start_height: curr_height, target_height: target_height, direction: mine_dir, regtest: u_config.regtest, mined_db_path: u_config.mined_db_path})

	}
}

