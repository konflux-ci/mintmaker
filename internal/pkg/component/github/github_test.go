package github

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Github Suite")
}

var _ = Describe("Module-level token cache variable", func() {

	Context("After the module initializes", func() {

		It("Should be possible to assign values to token cache with Set method", func() {

			testKey := "test_key"
			tokenInfo := TokenInfo{
				Token: "token_string", ExpiresAt: time.Now().Add(time.Hour),
			}

			ghAppInstallationTokenCache.Set(testKey, tokenInfo)

			result, _ := ghAppInstallationTokenCache.Get(testKey)
			Expect(result).Should(Equal(tokenInfo))
		})
	})
})
