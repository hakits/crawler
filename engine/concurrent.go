package engine

import (
	"github.com/go-redis/redis"
	"log"
)

type ConcurrentEngine struct {
	Scheduler        Scheduler
	WorkerCount      int
	ItemChan         chan Item
	RequestProcessor Processor
	RedisCli         *redis.Client
}

type Processor func(Request) (ParseResult, error)

type Scheduler interface {
	Submit(Request)
	WorkerChan() chan Request
	ReadyNotifier
	Run()
}

type ReadyNotifier interface {
	WorkerReady(chan Request)
}

func (c *ConcurrentEngine) Run(seeds ...Request) {
	out := make(chan ParseResult)

	c.Scheduler.Run()

	for _, r := range seeds {
		c.Scheduler.Submit(r)
	}

	for i := 0; i < c.WorkerCount; i++ { //According WorkerCount, concurrent all work
		c.createWorker(c.Scheduler.WorkerChan(), out, c.Scheduler)
	}

	for {
		result := <-out
		for _, item := range result.Items {
			log.Printf("Got item:%v", item)
			go func() {
				c.ItemChan <- item
			}()
		}

		for _, request := range result.Requests { //submit new request to workerChan
			go func(request Request) {
				add := c.RedisCli.SAdd("check_zhipin_url", request.Url)
				log.Println("..........", add.Val())
				if add.Val() == 1 {
					c.Scheduler.Submit(request)
				}
			}(request)
		}
	}
}

func (c *ConcurrentEngine) createWorker(in chan Request, out chan ParseResult, ready ReadyNotifier) {
	go func() {
		for {
			// tell scheduler i'm ready
			ready.WorkerReady(in)
			request := <-in
			result, err := c.RequestProcessor(request) //Worker(request) //call rpc
			if err != nil {
				continue
			}
			out <- result
		}
	}()

}
