package tier1

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path"
	"path/filepath"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob"

	"github.com/EnterpriseDB/klio/pkg/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service", Ordered, func() {
	var (
		ctx       context.Context
		ctxCancel context.CancelFunc
		cfg       *config.Data
		logger    *slog.Logger
		svc       Service
	)

	BeforeEach(func() {
		ctx, ctxCancel = context.WithCancel(context.Background()) //nolint:fatcontext
		tempDir, err := os.MkdirTemp("", "tier1")
		Expect(err).NotTo(HaveOccurred())
		cfg = &config.Data{
			Tier1: config.LocalArea{
				Path:     tempDir,
				Password: "password",
			},
			ClusterName: "test-cluster",
		}

		jsonKopiaCfg, err := json.Marshal(&repo.LocalConfig{
			Storage: &blob.ConnectionInfo{
				Type: "filesystem",
				Config: map[string]string{
					"path": path.Join(cfg.Tier1.Path, "data"),
				},
			},
			Caching: nil,
			ClientOptions: repo.ClientOptions{
				Hostname:                cfg.ClusterName,
				Username:                "root",
				Description:             "",
				EnableActions:           false,
				FormatBlobCacheDuration: 900000000000,
			},
		})

		Expect(err).ToNot(HaveOccurred())
		err = os.WriteFile(filepath.Join(cfg.Tier1.Path, "kopiacfg"), jsonKopiaCfg, 0o600)
		Expect(err).ToNot(HaveOccurred())

		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
		svc = New(cfg, logger)
	})

	AfterEach(func() {
		ctxCancel()
		err := os.RemoveAll(cfg.Tier1.Path)
		logger.Error("failed to remove temporary directory", "err", err)
	})

	It("can store and retrieve a WAL", func() {
		go func() {
			defer GinkgoRecover()
			err := svc.Serve(ctx)
			Expect(err).ToNot(HaveOccurred())
		}()

		Eventually(func(g Gomega) {
			impl, ok := svc.(*impl)
			g.Expect(ok).To(BeTrue())
			g.Expect(impl.repository).ToNot(BeNil())
		}).Should(Succeed())

		err := svc.StoreWAL(ctx, "test-wal", []byte("test-content"))
		Expect(err).ToNot(HaveOccurred())

		err = svc.StoreWAL(ctx, "test-wal-2", []byte("test-content-2"))
		Expect(err).ToNot(HaveOccurred())

		name, err := svc.GetLatestWALFileName(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(name).To(Equal("test-wal-2"))

		walEntry, err := svc.GetWAL(ctx, name)
		Expect(err).ToNot(HaveOccurred())
		Expect(walEntry.content).To(Equal([]byte("test-content-2")))
	})

	It("closes the repository when context is done", func() {
		go func() {
			defer GinkgoRecover()
			err := svc.Serve(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()

		Eventually(func(g Gomega) {
			g.Expect(svc.IsReady()).To(BeTrue())
		}).Should(Succeed())

		ctxCancel()

		Eventually(func(g Gomega) {
			g.Expect(svc.IsReady()).To(BeFalse())
		}).Should(Succeed())
	})

	It("should not panic if there is no repository initialized", func() {
		err := svc.StoreWAL(ctx, "test-wal", []byte("test-content"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("repository not initialized"))
	})
})
