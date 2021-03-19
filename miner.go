package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
	"github.com/syndtr/goleveldb/leveldb"
)

// ~~~COUNTER~~~

type Counter struct {
	sync.Mutex
	s int // start
	h int // height
	t int // target
	dir int // direction (mine: 1, unmine: -1)
}

func (c *Counter) tick() int {
	c.Lock()
	// If h is still within s->t go, else stop
	switch (c.h >= c.s && c.h <= c.t) {
	case true:
		output := c.h
		c.h = c.h + c.dir
		c.Unlock()
		return output
	default:
		c.Unlock()
		return -1
	}
}

func (c *Counter) check() int {
	c.Lock()
	x := c.h
	c.Unlock()
	return x
} 


// ~~~DATA TYPES~~~

// Mining run config
type MiningConfig struct {
	// RPC Log Info
	username string
	password string

	// Start and finish
	start_height int
	target_height int

	// Mine/Unmine
	direction int

	// Regtest
	regtest bool

	// DB Path
	mined_db_path string
}


// Output by callers
type Call_Output struct {
	height int
	hash string
	content interface{}
}

// Read_Block
type Read_Block struct{
	height int
	hash string
	content [][4]string
}


// ~~~INDEX~~~

type Index struct {

	// StopStart Marker. Routines check this; if false, stop. Only main routine modiufies
	run bool

	// Channel to the miner, reffed by readers
	to_miner chan Read_Block

	// Counters
	call_counter *Counter
	mine_counter *Counter

	// Regtest
	regtest bool

	// Direction
	direction int
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

	// Miner Emergency Channel
	e_chan chan int

}


func make_caller(in_user, in_pass string, e_chan chan int, index *Index) *Caller {
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
		e_chan: e_chan,
	}
	return caller
}


func (c Caller) make_bitcoin_call(method string, params string) interface{} {
	port := "8332"
	if c.index.regtest{
		port = "18443"
	}

	fmt.Println("MAKE CALL", method, params)
	// make post
	body := strings.NewReader("{\"jsonrpc\":\"1.0\",\"id\":\"curltext\",\"method\":\""+method+"\",\"params\":["+params+"]}")
	req, err := http.NewRequest("POST", "http://"+c.user+":"+c.pass+"@127.0.0.1:"+port, body)

	if err != nil {
		fmt.Println("phone btc ERROR", err)
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := c.http_client.Do(req)
	
	if err != nil {
		fmt.Println(fmt.Println("phone btc ERROR 2", err))
		log.Fatal(err)

	}

	defer resp.Body.Close()
	resp_bytes, err :=  ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(fmt.Println("phone btc ERROR 3", err))
		log.Fatal(err)

	}
	var result map[string]interface{}

	err = json.Unmarshal(resp_bytes, &result)
	if err != nil {
		fmt.Println("ERROR", err)
		log.Fatal(err)
	}

	return result["result"]
}


func (c Caller) get_block_info_for_height(height int) (map[string]interface{}, string) {

	// Get hash and remove \n
	hash := strings.TrimRight(fmt.Sprintf("%v", c.make_bitcoin_call("getblockhash", fmt.Sprint(height))), "\r\n")

	// Get Block 
	block_info := c.make_bitcoin_call("getblock", "\""+hash+"\", "+"2").(map[string]interface{})
	
	return block_info, hash
}


func (c Caller) run() {
	// Outer loop runs forever, inner loop only runs if set active

	// Cycle counter
	var x int

	for {
		// If run is off, stop running
		if !c.index.run {
			return
		}

		// Check for emergency calls from the miner
		select {
		case height := <- c.e_chan:

			//Run the emergency call
			out, hash := c.get_block_info_for_height(height)
			co := &Call_Output{height: height, hash: hash, content: out}
			go read_block(c.index, co)

		default:
		}

		// Every 200 cycles check if ahead by 500, if so, wait until caught up to 200
		if x >= 200 {
			x = 0
			if c.index.call_counter.check() > c.index.mine_counter.check() + 500*c.index.direction {
				for c.index.call_counter.check() > c.index.mine_counter.check() + 200*c.index.direction{

					// Check again for emergency calls
					select {
					case height := <- c.e_chan:

						//Run the emergency call
						out, hash := c.get_block_info_for_height(height)
						co := &Call_Output{height: height, hash: hash, content: out}
						go read_block(c.index, co)
			
					default:
					}

					time.Sleep(500 * time.Millisecond)
				}
			}
		}


		res := c.index.call_counter.tick()
		switch res {
		case -1:
			// STOP
			continue
		default:
			// Pull for res
			c.current_pull = res
		}

		out, hash := c.get_block_info_for_height(c.current_pull)
		co := &Call_Output{height: c.current_pull, hash: hash, content: out}
		go read_block(c.index, co)

		x++
	}
}



// ~~~READER~~~

func read_block(index *Index, input *Call_Output) {
	// Read the block
	block_output := p_get_all_P2WSH(input.height, input.content.(map[string]interface{}))
	block_output.hash = input.hash
	
	// Wait your turn, then mine
	for {

		// If run is off, stop running
		if !index.run {
			return
		}

		index.mine_counter.Lock()
		ch := index.mine_counter.h
		index.mine_counter.Unlock()

		switch {
		case ch < block_output.height:
			// Wait
			time.Sleep(5*time.Millisecond)
			continue
		case ch == block_output.height:
			// My turn
			index.to_miner <- block_output
			return
		case ch > block_output.height:
			// Orphaned
			log.Println("Orphaned Reader at height ", ch)
			return
		}
	}
}


func p_get_all_P2WSH(height int, block_json map[string]interface{}) Read_Block {


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
							//fmt.Println(scriptPubKey)
							hex := fmt.Sprintf("%v", scriptPubKey["hex"])
							ro := [4]string{
								strings.ToUpper(hex[4:]),
								fmt.Sprintf("%08d", height),
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
	output := Read_Block{
		height: height,
		content: add_array,
	}
	return output
}


// ~~~MINER~~~

type Miner struct {
	index *Index
	in_chan chan Read_Block
	e_chan chan int
	hash_db *leveldb.DB
}

func (m Miner) mine(input Read_Block) {

	content := input.content


	for i := range content {
		url := "/mining/mine/"+content[i][0]+"/"+content[i][1]+content[i][2]+content[i][3]
		p_make_haircomb_call(url, false)
	}

	// Flush
	p_make_haircomb_call("/mining/mine/FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF/9999999999999999", false)

	// Commit hash to DB
	err := m.hash_db.Put([]byte(fmt.Sprint(input.height)), []byte(input.hash), nil)
	if err != nil {
		log.Fatal("HASH STORE ERROR", err)
	}

	fmt.Println("MINED:", m.index.mine_counter.h)
	
}

func (m Miner) run() {

	// Emergency counter
	x := time.Now()

	for {
		// If run is off, stop running
		if !m.index.run {
			return
		}
		
		select {
		case inc := <-m.in_chan:
			// Mine the block
			m.mine(inc)

			// Increment counter
			m.index.mine_counter.tick()
			
			// Reset emergency counter
			x = time.Now()

		default:
			if time.Since(x) > 30*time.Second{
				fmt.Println("EMERGENCY PULL", m.index.mine_counter.check())
				// Make an emergency pull request for the current height
				m.e_chan <- m.index.mine_counter.check()
				x = time.Now()
			}
		}
	}
}

func p_make_haircomb_call(url string, wait bool) string {
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


func mine(config MiningConfig) int {

	// Load the DB
	db, err := leveldb.OpenFile(config.mined_db_path, nil)
	if err != nil {
		log.Fatal("DB open error", err)
	}
	defer db.Close()

	// Make the counters
	call_counter := &Counter{
		s: config.start_height,
		h: config.start_height,
		t: config.target_height,
		dir: config.direction,
	}
	mine_counter := &Counter{
		s: config.start_height,
		h: config.start_height,
		t: config.target_height,
		dir: config.direction,
	}

	// Make the index
	index := &Index{
		call_counter: call_counter,
		mine_counter: mine_counter,
		to_miner: make(chan Read_Block),
		run: true,
		regtest: config.regtest,
		direction: config.direction,
	}
	
	// Make the miner
	miner := &Miner{
		index: index,
		in_chan: index.to_miner,
		e_chan: make(chan int),
		hash_db: db,
	}
	go miner.run()
		
	// Make callers
	callers := []*Caller{}
	for x:= 1; x <= 6; x++ {
		callers = append(callers, make_caller(config.username, config.password, miner.e_chan, index))
	}
	
	// Run concurrent
	for x := range callers {
		callers[x].active = true
		go callers[x].run()
	}

	time_start := time.Now().UnixNano()
	for {
		index.mine_counter.Lock()
		if index.mine_counter.h == index.mine_counter.t+index.mine_counter.dir{
			index.mine_counter.Unlock()
			break
		}
		index.mine_counter.Unlock()
		time.Sleep(time.Millisecond)
	}

	fmt.Println("DONE")
	fmt.Println("TIME:", float64(time.Now().UnixNano() - time_start)/float64(1000000000), "seconds")
	return index.mine_counter.t
}	