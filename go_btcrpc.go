package main

import (
	"bytes"
	"fmt"
	"encoding/json"
	"net/http"
	"io/ioutil"
	//"strconv"
	"strings"
	"time"

	//"log"
	"os/exec"
)

var user = "user"
var password = "password"

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

func make_bitcoin_call_cli(values...string) string{
	// run the btc command line

	// Define paths
	var cli = "Y:/Storage/BTC_Blockchain/realBTC/Bitcoin/daemon/bitcoin-cli.exe"

	cmd := exec.Command(cli, values...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()


	if err != nil {
		fmt.Println(fmt.Sprint(err) + ": " + stderr.String())
	}

	return out.String()
}

func make_bitcoin_call(client *http.Client, method string, params string) interface{} {

	fmt.Println("MAKE CALL", method, params)
	// make post
	body := strings.NewReader("{\"jsonrpc\":\"1.0\",\"id\":\"curltext\",\"method\":\""+method+"\",\"params\":["+params+"]}")
	req, err := http.NewRequest("POST", "http://"+user+":"+password+"@127.0.0.1:8332", body)


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


func get_all_P2WSH(block_json map[string]interface{}, reorg bool) {

	txes := block_json["tx"].([]interface{})
	block_height := int(block_json["height"].(float64))
	fmt.Println(block_height)

	// For each TX...
	for i := range txes {

		// ...Check all outputs for new P2WSH
		this_tx := txes[i].(map[string]interface{})
		mine_tx_P2WSHes(this_tx, block_height, i, reorg)
		
	}
}

 func mine_tx_P2WSHes(tx_info map[string]interface{}, block_height, txno int, reorg bool) {
 	vout := tx_info["vout"].([]interface{})
	for i := range vout {
		this_out := vout[i].(map[string]interface{})

		// If it has a scriptPubKey
		if this_out["scriptPubKey"] != nil{
			scriptPubKey := this_out["scriptPubKey"].(map[string]interface{})

			// If it has type
			if scriptPubKey["type"] != nil{
				my_type := scriptPubKey["type"].(string)
				
				// If type is "witness_v0_scripthash"
				if my_type == "witness_v0_scripthash"{

					// Pull the hex
					if scriptPubKey["hex"] != nil{
						hex := fmt.Sprintf("%v", scriptPubKey["hex"])

						// Format and mine
						if reorg {
							// Not sure how to do this yet
							fmt.Println("REORG")
							url := "/mining/mine/"+strings.ToUpper(hex[4:])+"/"+fmt.Sprintf("%08d%04d%04d", 50000000+block_height, txno, i)
							make_haircomb_call(url, false)
						} else {
							url := "/mining/mine/"+strings.ToUpper(hex[4:])+"/"+fmt.Sprintf("%08d%04d%04d", block_height, txno, i)
							make_haircomb_call(url, false)
						}
					}
				}
			}
		}
	} 
} 

func format_entry( hex string, height, txno, voutno int) [2]string {
	var output [2]string

	//Format the hex by removing the 0020, and include
	output[0] = strings.ToUpper(hex[4:])
	output[1] = fmt.Sprintf("%08d%04d%04d", height, txno, voutno)
	return output
}

func mine_block(block [][2]string) {
	for i := range block {
		url := "/mining/mine/"+block[i][0]+"/"+block[i][1]
		make_haircomb_call(url, false)
	}
}


func reorg_check(client *http.Client, hc_height int) bool {
	// Reference the mined blocks db for the most recent block hash, and compare it against the hash of the block in the BTC chain
	if get_hc_block_hash(hc_height) == strings.TrimRight(fmt.Sprintf("%v", make_bitcoin_call(client, "getblockhash", fmt.Sprint(hc_height))), "\r\n") {
		return false
	} 
	return true
}

func get_hc_block_hash(height int) string {
	// Pull from the stored DB of HC's mined blocks
	return "hash_here"
}
/*
func main() {
	// SETUP

	// make the http client
	http_client := make_client()

	x := 0 
	for {
		x++
		fmt.Println("GO", x)
		make_bitcoin_call(http_client, "getblock", "\"000000000000000000029c803d6802a05c73bee470b77749d1cee070230adfd1\", "+"2")
	}

	// ping haircomb for highest known block
	base_height := make_haircomb_call("/height/get", true)
	fmt.Println(base_height)

	// format curr_height
	curr_height, err := strconv.Atoi(base_height)
	if err != nil {
		fmt.Println("stringtoint ERROR", err)
	}

	// move currheight to first comb block (481824)
	if curr_height < 481824 {
		curr_height = 481824
	}


	// RUN OUTER LOOP
	for {
		// wait 5 seconds
		time.Sleep(5 * time.Second)

		// Check for reorg 
		/*if reorg_check(http_client, curr_height) {
			// Find the reorged block and set curr_height to it
				// Find by going back and comparing mined block hashes against BTC block hashes. The earliest difference is the reorged block.

			// Process for reorg , not sure how to do this yet
		}

		// Pull the current BTC height
		btc_height := int(make_bitcoin_call(http_client, "getblockcount", "").(float64))

		// If caught up, skip this cycle
		if btc_height == curr_height {
			continue
		}

		// RUN INNER LOOP
		for {
			fmt.Println("Pulling for", curr_height) 

			// Get hash and remove \n
			hash := strings.TrimRight(fmt.Sprintf("%v", make_bitcoin_call(http_client, "getblockhash", fmt.Sprint(curr_height))), "\r\n")

			// Get Block 
			block := make_bitcoin_call(http_client, "getblock", "\""+hash+"\", "+"2").(map[string]interface{})
			
			fmt.Println(curr_height, "Pulled")

			// Mine all the block (block)
			get_all_P2WSH(block, false)

			// Mine a flush
			make_haircomb_call("/mining/mine/FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF/9999999999999999", false)

			fmt.Println(curr_height, "Mined")

			// Increment Height, repeat if not btc_height reached
			curr_height+=1
			if curr_height == btc_height {
				break
			}
		}
	}
}*/

