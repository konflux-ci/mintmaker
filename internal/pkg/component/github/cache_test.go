package github

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Cache Suite")
}

var _ = Describe("TokenCache object", func() {

	Context("When Setter method is called for the first time", func() {

		It("Should write entry successfully, without 'nil map' error", func() {

			var tokenCache TokenCache
			testKey := "test_key"
			tokenInfo := TokenInfo{
				Token: "token_string", ExpiresAt: time.Now().Add(time.Hour),
			}

			tokenCache.Set(testKey, tokenInfo)

			result, _ := tokenCache.Get(testKey)
			Expect(result).Should(Equal(tokenInfo))))
		})

	})
})
