package github

import (
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("StaleAllowedCache", func() {
	var (
		refreshCount    int
		refreshFunc     func() (interface{}, error)
		refreshError    error
		refreshData     interface{}
		refreshDuration time.Duration
		cache           *StaleAllowedCache
		refreshMutex    sync.Mutex
	)

	BeforeEach(func() {
		refreshCount = 0
		refreshError = nil
		refreshData = "test-data"
		refreshDuration = 100 * time.Millisecond

		refreshFunc = func() (interface{}, error) {
			refreshMutex.Lock()
			defer refreshMutex.Unlock()
			refreshCount++
			if refreshError != nil {
				return nil, refreshError
			}
			return refreshData, nil
		}

		cache = NewStaleAllowedCache(refreshDuration, refreshFunc)
	})

	Context("when accessing cache for the first time", func() {
		It("should call the refresh function and return the data", func() {
			data, ok := cache.Get("test-key")

			Expect(ok).To(BeTrue())
			Expect(data).To(Equal(refreshData))
			Expect(refreshCount).To(Equal(1))
		})

		It("should return false if refresh function returns an error", func() {
			refreshError = errors.New("refresh error")

			data, ok := cache.Get("test-key")

			Expect(ok).To(BeFalse())
			Expect(data).To(BeNil())
			Expect(refreshCount).To(Equal(1))
		})
	})

	Context("when accessing cached data", func() {
		BeforeEach(func() {
			// Initialize the cache, after that the refreshCount is 1
			_, _ = cache.Get("test-key")
		})

		It("should return cached data without calling refresh function if not expired", func() {
			data, ok := cache.Get("test-key")

			Expect(ok).To(BeTrue())
			Expect(data).To(Equal(refreshData))
			Expect(refreshCount).To(Equal(1))
		})

		It("should call refresh function in background when data is expired", func() {
			// Wait for cache to expire
			time.Sleep(refreshDuration + 10*time.Millisecond)

			// Change the refreshData to verify we get the old data first
			oldData := refreshData
			refreshData = "new-data"

			// Add a delay to the refresh function to simulate a slow refresh
			originalRefreshFunc := refreshFunc
			refreshFunc = func() (interface{}, error) {
				time.Sleep(50 * time.Millisecond)
				return originalRefreshFunc()
			}

			// First access should return stale data but trigger refresh
			data, ok := cache.Get("test-key")

			Expect(ok).To(BeTrue())
			Expect(data).To(Equal(oldData)) // Should get old data

			// Wait for background refresh to complete
			time.Sleep(100 * time.Millisecond)

			// Second access should get new data
			data, ok = cache.Get("test-key")

			Expect(ok).To(BeTrue())
			Expect(data).To(Equal("new-data"))
			Expect(refreshCount).To(Equal(2))
		})
	})

	Context("when multiple goroutines access the cache simultaneously", func() {
		It("should only perform one refresh when multiple gets are called concurrently", func() {
			// Add delay to refresh function to ensure concurrency
			originalRefreshFunc := refreshFunc
			refreshFunc = func() (interface{}, error) {
				time.Sleep(50 * time.Millisecond)
				return originalRefreshFunc()
			}

			// Launch multiple goroutines to access the cache
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					data, ok := cache.Get("test-key")
					Expect(ok).To(BeTrue())
					Expect(data).To(Equal(refreshData))
				}()
			}
			wg.Wait()

			// Should only have called refresh once
			Expect(refreshCount).To(Equal(1))
		})
	})

	Context("when different keys are accessed", func() {
		It("should store and retrieve data for different keys", func() {
			// Set up different refresh functions for different keys
			refreshFunc = func() (interface{}, error) {
				refreshMutex.Lock()
				defer refreshMutex.Unlock()
				refreshCount++

				// Return data based on the key
				return "data", nil
			}

			cache = NewStaleAllowedCache(refreshDuration, refreshFunc)

			// Access first key
			data1, ok := cache.Get("key1")
			Expect(ok).To(BeTrue())
			Expect(data1).To(Equal("data"))

			// Access second key
			data2, ok := cache.Get("key2")
			Expect(ok).To(BeTrue())
			Expect(data2).To(Equal("data"))

			// Both keys should have triggered a refresh
			Expect(refreshCount).To(Equal(2))
		})
	})

	Context("when a refresh is in progress", func() {
		It("should wait for refresh to complete when no stale data is available", func() {
			// Set up a refresh function with a long delay
			refreshFunc = func() (interface{}, error) {
				time.Sleep(100 * time.Millisecond)
				refreshMutex.Lock()
				defer refreshMutex.Unlock()
				refreshCount++
				return "slow-data", nil
			}

			cache = NewStaleAllowedCache(refreshDuration, refreshFunc)

			// Start first access in a goroutine
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				start := time.Now()
				data, ok := cache.Get("key")
				duration := time.Since(start)

				Expect(ok).To(BeTrue())
				Expect(data).To(Equal("slow-data"))
				Expect(duration).To(BeNumerically(">=", 100*time.Millisecond))
			}()

			// Start second access, this should wait for the first refresh to complete
			time.Sleep(10 * time.Millisecond) // Give the first goroutine some time to start
			start := time.Now()
			data, ok := cache.Get("key")
			duration := time.Since(start)

			Expect(ok).To(BeTrue())
			Expect(data).To(Equal("slow-data"))
			// The first goroutine takes at least 100ms to refresh the cache,
			// we started 10ms later, so it should take at least 90ms for the
			// second access to get the cache data
			Expect(duration).To(BeNumerically(">=", 90*time.Millisecond))

			wg.Wait()
			Expect(refreshCount).To(Equal(1))
		})
	})
})
