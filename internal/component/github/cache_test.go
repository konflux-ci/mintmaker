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
		It("should return cached data without calling refresh function if not expired", func() {
			// Use a local counter to avoid interference from background refreshes
			// in other tests that might still be running.
			var localRefreshCount int
			var localMutex sync.Mutex

			localRefreshFunc := func() (interface{}, error) {
				localMutex.Lock()
				defer localMutex.Unlock()
				localRefreshCount++
				return refreshData, nil
			}
			localCache := NewStaleAllowedCache(refreshDuration, localRefreshFunc)

			// Initialize the cache
			_, _ = localCache.Get("test-key")
			Expect(localRefreshCount).To(Equal(1))

			// Second access should not trigger refresh
			data, ok := localCache.Get("test-key")

			Expect(ok).To(BeTrue())
			Expect(data).To(Equal(refreshData))
			Expect(localRefreshCount).To(Equal(1))
		})

		It("should call refresh function in background when data is expired", func() {
			// Use a local counter to avoid interference from background refreshes
			// in other tests that might still be running.
			var localRefreshCount int
			var localMutex sync.Mutex
			localRefreshData := "test-data"

			localRefreshFunc := func() (interface{}, error) {
				time.Sleep(50 * time.Millisecond)
				localMutex.Lock()
				defer localMutex.Unlock()
				localRefreshCount++
				return localRefreshData, nil
			}
			localCache := NewStaleAllowedCache(refreshDuration, localRefreshFunc)

			// Initialize the cache
			_, _ = localCache.Get("test-key")
			Expect(localRefreshCount).To(Equal(1))

			// Wait for cache to expire
			time.Sleep(refreshDuration + 10*time.Millisecond)

			// Change the refresh data to verify we get the old data first
			oldData := localRefreshData
			localRefreshData = "new-data"

			// First access should return stale data but trigger background refresh
			data, ok := localCache.Get("test-key")

			Expect(ok).To(BeTrue())
			Expect(data).To(Equal(oldData)) // Should get old data

			// Poll until background refresh completes and new data is available
			Eventually(func() interface{} {
				data, _ := localCache.Get("test-key")
				return data
			}, 500*time.Millisecond, 10*time.Millisecond).Should(Equal("new-data"))

			localMutex.Lock()
			Expect(localRefreshCount).To(Equal(2))
			localMutex.Unlock()
		})
	})

	Context("when multiple goroutines access the cache simultaneously", func() {
		It("should only perform one refresh when multiple gets are called concurrently", func() {
			// Use a local counter to avoid interference from background refreshes
			// in other tests that might still be running.
			var localRefreshCount int
			var localMutex sync.Mutex

			// Create a new cache with a slow refresh function to ensure concurrency
			slowRefreshFunc := func() (interface{}, error) {
				time.Sleep(50 * time.Millisecond)
				localMutex.Lock()
				defer localMutex.Unlock()
				localRefreshCount++
				return refreshData, nil
			}
			localCache := NewStaleAllowedCache(refreshDuration, slowRefreshFunc)

			// Launch multiple goroutines to access the cache.
			// All goroutines will enter getWithRefresh() and either:
			// 1. Be the first one and perform the refresh, or
			// 2. Wait on refreshInProgress and then return the cached data
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					data, ok := localCache.Get("test-key")
					Expect(ok).To(BeTrue())
					Expect(data).To(Equal(refreshData))
				}()
			}
			wg.Wait()

			// Should only have called refresh once
			Expect(localRefreshCount).To(Equal(1))
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
			var started sync.WaitGroup
			started.Add(1)

			// Set up a refresh function with a long delay
			refreshFunc = func() (interface{}, error) {
				started.Done() // Signal that refresh has started
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
				data, ok := cache.Get("key")

				Expect(ok).To(BeTrue())
				Expect(data).To(Equal("slow-data"))
			}()

			// Wait until the first goroutine has actually entered the refresh function,
			// rather than relying on a fixed sleep
			started.Wait()

			// Second access should block on the in-progress refresh
			data, ok := cache.Get("key")

			Expect(ok).To(BeTrue())
			Expect(data).To(Equal("slow-data"))

			wg.Wait()
			Expect(refreshCount).To(Equal(1))
		})
	})
})
