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

// Output by Readers
type Read_Output struct { 
	height int
	content [][4]string
}


// ~~~CALLER~~~

type Caller struct {
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


func make_caller(in_user, in_pass string, counter *Counter, out_chan chan Call_Output) *Caller {
	in_http_client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
		},
		Timeout: 300 * time.Second,
	}
	caller := &Caller{
		user: in_user,
		pass: in_pass,
		http_client: in_http_client,
		global_counter: counter,
		out_chan: out_chan,

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

		fmt.Println("INNER LOOP")
		// Lock counter
		c.global_counter.mu.Lock()

		// If curr height is at target, stop
		if c.global_counter.h >= c.global_counter.t {
			fmt.Println("IM DONE")
			c.global_counter.mu.Unlock()
			c.active = false
			continue
		}

		// Get my next pull and increment counter
		c.current_pull = c.global_counter.h
		c.global_counter.h++
		c.global_counter.mu.Unlock()
		
		// Pull
		fmt.Println("CALLER PUSHING OUT")
		c.out_chan <- Call_Output{height: c.current_pull, content: c.get_block_info_for_height(c.current_pull)}

	}

}


// ~~~READER~~~

type Reader struct {

	// Input Channel (C2R -> Reader)
	in_chan chan Call_Output

	// Output Channel (Reader -> R2M)
	rmanager *RManager
}

func make_reader(rmanager *RManager) *Reader {
	fmt.Println("READER MADE")
	reader := &Reader{
		in_chan: make(chan Call_Output),
		rmanager: rmanager,
	}
	return reader
}

func (r Reader) get_all_P2WSH(height int, block_json map[string]interface{}) *Read_Output {

	add_array := [][4]string{}

	txes := block_json["tx"].([]interface{})
	//block_height := int(block_json["height"].(float64))

	// For each TX...
	for i := range txes {

		// ...Check all outputs for P2WSH
		tx_info := txes[i].(map[string]interface{})
		vout := tx_info["vout"].([]interface{})
		for x := range vout {
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
								fmt.Sprintf("%04d", i),
								fmt.Sprintf("%04d", x), 
							}
							add_array = append(add_array, ro)
						}
					}
				}
			}
		}
	} 	
	output := &Read_Output{
		height: height,
		content: add_array,
	}
	return output
}


func (r Reader) run() {
	input := <-r.in_chan
	fmt.Println("READER GOT INPUT")
	block_output := r.get_all_P2WSH(input.height, input.content.(map[string]interface{}))
	r.rmanager.in_chan <- block_output
}


// ~~~C2R_FUNNEL~~~

type C2R_Funnel struct {
	
	// Status
	active bool

	// Slice of callers
	callers []*Caller

	// Callers put info here
	in_chan chan Call_Output 

	// Ref to the miner, to give to readers on creation
	rmanager *RManager

	// Message Channels (Talks to Main)
	msg_in chan Message
	msg_out chan Message
	

}


func make_c2r_funnel(in_chan chan Call_Output, callers []*Caller, rmanager *RManager) *C2R_Funnel {
	c2r_funnel := &C2R_Funnel{
		in_chan: in_chan,
		callers: callers,
		rmanager: rmanager,
		msg_in: make(chan Message),
		msg_out: make(chan Message),
	}
	return c2r_funnel
}

func (f C2R_Funnel) run() {
	for {
		
		// Check messages
		select {
		case x, ok := <-f.msg_in:
			if ok {
				fmt.Println("msg1", x, ok)
				msg := x
				switch msg.msg {
				case "quit":
					return
				case "onoff":
					switch msg.val {
					case 0:
						f.active = false
					case 1:
						f.active = true
					}
				}
			} else {
				fmt.Println("msg2", x, ok)
			}
		default:
			// No incoming
		}
			
		

		// If not active, sleep then skip
		if !f.active {
			time.Sleep(200*time.Millisecond)
			continue
		}
		
		// Receive From Callers
		select {
		case x, ok := <-f.in_chan:
			if ok {
				fmt.Println("C2R_FUNNEL PULL IN")
				// Make a new reader, start, add to list of active readers
				new_reader := make_reader(f.rmanager)
				go new_reader.run()
				new_reader.in_chan <- x
			} else {
				fmt.Println("x2", ok)
			}
		default:
			// No incoming
		}

	}
}

// ~~~RManager~~~

type RManager struct {

	// Readers dump here
	in_chan chan *Read_Output

	// Counter, used for subroutines
	rm_counter *RM_Counter

	// Miner ref
	miner *Miner

}

type RM_Counter struct {

	// Lock
	mu sync.Mutex

	// Current Height
	mined_height int

	// Target Height
	target_height int
	
}

func make_rmanager(counter *Counter) *RManager {
	counter.mu.Lock()
	rm_counter := &RM_Counter{
		mined_height: counter.h,
		target_height: counter.t,
	}
	miner := &Miner{
		in_chan: make(chan [][4]string),
	}
	counter.mu.Unlock()
	rmanager := &RManager{
		in_chan: make(chan *Read_Output),
		rm_counter: rm_counter,
		miner: miner,
		
	}
	return rmanager
}

func (rm RManager) run() {

	// Spawn new goroutine for each pull
	go rm_instance(<-rm.in_chan, rm.rm_counter, rm.miner)
}

func rm_instance(input *Read_Output, rm_counter *RM_Counter, miner *Miner) {

	
	// Wait until its my turn, then initiate mining
	for {
		rm_counter.mu.Lock()
		ch := rm_counter.mined_height
		rm_counter.mu.Unlock()
		switch {
		case ch < input.height:
			continue
		case ch == input.height:
			miner.in_chan <- input.content
		}
		rm_counter.mu.Lock()
		rm_counter.mu.Unlock()
	}
}

// ~~~MINER~~~

type Miner struct {
	in_chan chan [][4]string
}

func make_miner() *Miner {
	miner := &Miner{
		in_chan: make(chan [][4]string),
	}
	return miner
}

func (m Miner) mine(input [][4]string) {
	for i := range input {
		url := "/mining/mine/"+input[i][0]+"/"+input[i][1]+input[i][2]+input[i][3]
		//make_haircomb_call(url, false)
		fmt.Printf(url)
	}
}

func (m Miner) run() {
	m.mine(<-m.in_chan)
}




func main() {
	fmt.Println("1")

	// Make the counter
	global_counter := Counter{
		h: 555550,
		t: 555600,
	}

	// Make caller output channel
	caller_output_channel := make(chan Call_Output, 1)
	fmt.Println("2")

	// Make callers
	callers := []*Caller{}
	for x:= 1; x <= 5; x++ {
		callers = append(callers, make_caller("user", "password", &global_counter, caller_output_channel))
	}
	fmt.Println("3")

	// Make rm_manager
	rmanager := make_rmanager(&global_counter)
	go rmanager.run()

	// Make c2r_funnel
	c2r_funnel := make_c2r_funnel(caller_output_channel, callers, rmanager)
	
	go c2r_funnel.run()
	fmt.Println("4")
	c2r_funnel.msg_in <- Message{"onoff", 1}

	
	// Run a non-concurrent test
	callers[0].active = true
	go callers[0].run()
	
	

	
	// Run a concurrent test
	/*for x := range callers {
		callers[x].active = true
		go callers[x].run()
	}*/
	//var my_time int
	time_start := time.Now().UnixNano()
	for {
		//time.Sleep(5*time.Second)
		//fmt.Println("MAIN LOOP")
		//fmt.Println(len(output_channel))
		/*
		global_counter.mu.Lock()
		if global_counter.h >= target_height{
			global_counter.mu.Unlock()
			break
		}
		global_counter.mu.Unlock()*/
		//my_time++
		time.Sleep(time.Second)
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

