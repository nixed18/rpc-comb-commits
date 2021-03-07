package main

import (
	"bytes"
	"fmt"
	"encoding/json"
	"net/http"
	"io/ioutil"
	"strconv"
	"strings"

	//"log"
	"os/exec"
)

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
func make_bitcoin_call(values...string) string {
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

func get_all_P2WSH(block_json string) [][2]string {
	var decoded map[string]interface{}
	err := json.Unmarshal([]byte(block_json), &decoded)

	if err != nil{
		fmt.Println("JSON ERROR", err)
	}

	txes := decoded["tx"].([]interface{})
	var new_P2WSHes [][2]string
	block_height := int(decoded["height"].(float64))
	fmt.Println(block_height)

	// For each TX...
	for i := range txes {

		// ...Check all outputs for new P2WSH
		this_tx := txes[i].(map[string]interface{})
		result := scan_tx_for_P2WSH(this_tx, block_height, i)

		// If some amount of new P2WSH were found...
		if result != nil {
			// ...Stick them all on the end as individual entries
			new_P2WSHes = append(new_P2WSHes, result...)
		}	
	}
	return new_P2WSHes

}

 func scan_tx_for_P2WSH(tx_info map[string]interface{}, block_height, txno int) [][2]string {
	var res [][2]string
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

						// Format the entry
						full_entry := format_entry(hex, block_height, txno, i)

						// Append to curr tx result
						res = append(res, full_entry)
						
					}
				}
			}
		}
	} 
	
	return res 
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

func main() {

	// SETUP

	// ping haircomb for highest known block.
 	base_height := make_haircomb_call("/height/get", true)
	fmt.Println(base_height)

	//format curr_height
	curr_height, err := strconv.Atoi(base_height)
	if err != nil {
		fmt.Println("stringtoint ERROR", err)
	}

	// move currheight to first comb block (481824)
	if curr_height < 481824 {
		curr_height = 481824
	}

	// RUN
	for {
		fmt.Println("Pulling for", curr_height) 

		// Get hash and remove \n
		hash := strings.TrimRight(make_bitcoin_call("getblockhash", fmt.Sprint(curr_height)), "\r\n")
		fmt.Println(hash)


		// Get Block 
		block := make_bitcoin_call("getblock", hash, "2")

		//get_all_new_P2WSH(block)
		blocks_P2WSH := get_all_P2WSH(block)

		// Add a flush
		blocks_P2WSH = append(blocks_P2WSH, [2]string{"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", "9999999999999999"})

		// Mine Block
		mine_block(blocks_P2WSH)

		// Increment Height, repeat
		curr_height+=1
		fmt.Println(blocks_P2WSH)
	}


}