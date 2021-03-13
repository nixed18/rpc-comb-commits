package main

import (
	"fmt"
	"io/ioutil"
	//"log"
	"net/http"
	"strings"
	"encoding/json"
	"time"
	"sync"
)

// Haircomb Call

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

// ~~~COUNTER~~~

type Counter struct {
	mu sync.Mutex
	h int // Height
	t int // target
}


// ~~~DATA TYPES~~~

// Used to communicate with running objects
type Message struct {
	msg string
	val int
}

// Output by callers
type Call_Output struct {
	height int
	content interface{}
}

// Read_Block
type Read_Block struct{
	height int
	content [][4]string
}


// ~~~INDEX~~~

type Index struct {
	// Channel to the miner, reffed by readers
	to_miner chan [][4]string

	// Counters
	call_counter *Counter
	mine_counter *Counter
}


// ~~~CALLER~~~

type Caller struct {

	index *Index

	// Log info
	user string
	pass string

	// Status
	active bool

	// HTTP
	http_client *http.Client

	// Current Block Pull
	current_pull int

	// Global current height counter reference
	global_counter *Counter

	// Output Channel Reference
	out_chan chan Call_Output

	// Message Channels
	msg_in chan Message
	msg_out chan Message

}


func make_caller(in_user, in_pass string, index *Index) *Caller {
	in_http_client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
		},
		Timeout: 300 * time.Second,
	}
	caller := &Caller{
		index: index,
		user: in_user,
		pass: in_pass,
		http_client: in_http_client,
		msg_in: make(chan Message),
		msg_out: make(chan Message),
	}
	return caller
}

func (c Caller) make_bitcoin_call(method string, params string) interface{} {

	fmt.Println("MAKE CALL", method, params)
	// make post
	body := strings.NewReader("{\"jsonrpc\":\"1.0\",\"id\":\"curltext\",\"method\":\""+method+"\",\"params\":["+params+"]}")
	req, err := http.NewRequest("POST", "http://"+c.user+":"+c.pass+"@127.0.0.1:8332", body)

	if err != nil {
		fmt.Println("phone btc ERROR", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := c.http_client.Do(req)
	
	if err != nil {
		fmt.Println(fmt.Println("phone btc ERROR 2", err))

	}

	defer resp.Body.Close()
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

func (c Caller) get_block_info_for_height(height int) map[string]interface{} {

	// Get hash and remove \n
	hash := strings.TrimRight(fmt.Sprintf("%v", c.make_bitcoin_call("getblockhash", fmt.Sprint(height))), "\r\n")

	// Get Block 
	block_info := c.make_bitcoin_call("getblock", "\""+hash+"\", "+"2").(map[string]interface{})
	
	return block_info
}

func (c Caller) run() {
	// Outer loop runs forever, inner loop only runs if set active

	for {

		// Check messages
		for len(c.msg_in) > 0{
			msg := <-c.msg_in
			switch msg.msg {
			case "quit":
				return
			case "onoff":
				switch msg.val {
				case 0:
					c.active = false
				case 1:
					c.active = true
				}
			}
		}

		// If not active, sleep then skip
		if !c.active {
			time.Sleep(200*time.Millisecond)
			continue
		}

		//fmt.Println("INNER LOOP")
		// Lock counter
		c.index.call_counter.mu.Lock()

		// If curr height is at target, stop
		if c.index.call_counter.h >= c.index.call_counter.t {
			//fmt.Println("IM DONE")
			c.index.call_counter.mu.Unlock()
			c.active = false
			continue
		}

		// Get my next pull and increment counter
		c.current_pull = c.index.call_counter.h
		c.index.call_counter.h++
		c.index.call_counter.mu.Unlock()
		
		// Pull
		//fmt.Println("CALLER PUSHING OUT")

		co := &Call_Output{height: c.current_pull, content: c.get_block_info_for_height(c.current_pull)}
		
		go read_block(c.index, co)

	}

}


// ~~~READER~~~

func read_block(index *Index, input *Call_Output) {
	// Read the block
	block_output := get_all_P2WSH(input.height, input.content.(map[string]interface{}))
	
	// Wait your turn, then mine
	for {
		index.mine_counter.mu.Lock()
		ch := index.mine_counter.h
		index.mine_counter.mu.Unlock()
		//fmt.Println(block_output.height)
		switch {
		case ch < block_output.height:
			continue
		case ch == input.height:
			index.to_miner <- block_output.content
			return
		}
	}
}

func get_all_P2WSH(height int, block_json map[string]interface{}) *Read_Block {

	add_array := [][4]string{}

	txes := block_json["tx"].([]interface{})
	//block_height := int(block_json["height"].(float64))

	// For each TX...
	for x := range txes {

		// ...Check all outputs for P2WSH
		tx_info := txes[x].(map[string]interface{})
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
							ro := [4]string{
								strings.ToUpper(hex[4:]),
								fmt.Sprintf("%04d", height),
								fmt.Sprintf("%04d", x),
								fmt.Sprintf("%04d", i), 
							}
							add_array = append(add_array, ro)
						}
					}
				}
			}
		}
	} 	
	output := &Read_Block{
		height: height,
		content: add_array,
	}
	return output
}


// ~~~MINER~~~

type Miner struct {
	index *Index
	in_chan chan [][4]string
}

func (m Miner) mine(input [][4]string) {
	for i := range input {
		url := "/mining/mine/"+input[i][0]+"/"+input[i][1]+input[i][2]+input[i][3]
		//make_haircomb_call(url, false)
		fmt.Sprint(url)
	}

	if len(input) > 0 {
		fmt.Println("MINED:", input[0][1])
	}
}

func (m Miner) run() {
	for {
		m.mine(<-m.in_chan)
		m.index.mine_counter.mu.Lock()
		m.index.mine_counter.h++
		m.index.mine_counter.mu.Unlock()

	}
}


func main() {
	// Make the counters
	call_counter := &Counter{
		h: 555550,
		t: 555600,
	}
	mine_counter := &Counter{
		h: 555550,
		t: 555600,
	}

	// Make the index
	index := &Index{
		call_counter: call_counter,
		mine_counter: mine_counter,
		to_miner: make(chan [][4]string),
	}
	

	// Make callers
	callers := []*Caller{}
	for x:= 1; x <= 5; x++ {
		callers = append(callers, make_caller("user", "password", index))
	}

	// Make the miner
	miner := &Miner{
		index: index,
		in_chan: index.to_miner,
	}
	go miner.run()
	
	// Run a non-concurrent test
	/*callers[0].active = true
	go callers[0].run()*/
	
	
	// Run a concurrent test
	for x := range callers {
		callers[x].active = true
		go callers[x].run()
	}

	//var my_time int
	time_start := time.Now().UnixNano()
	for {
		//time.Sleep(5*time.Second)
		//fmt.Println("MAIN LOOP")
		//fmt.Println(len(output_channel))
		
		index.mine_counter.mu.Lock()
		if index.mine_counter.h >= index.mine_counter.t{
			index.mine_counter.mu.Unlock()
			break
		}
		index.mine_counter.mu.Unlock()
		//my_time++
		time.Sleep(time.Millisecond)
	}

	fmt.Println("DONE")
	fmt.Println("TIME:", (time.Now().UnixNano() - time_start)/10000)
	/*
	// Run a concurrent test
	// Re-assign current_height
	current_height = 555555

	// Boot all callers
	for x := range callers {
		callers[x].run()
	}
	*/

}	

