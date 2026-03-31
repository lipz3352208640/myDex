package solmodel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const defaultTestDSN = "root:123456@tcp(47.76.194.205:3306)/fun_dex?parseTime=true&timeout=5s&readTimeout=5s&writeTimeout=5s"

func TestConnect(t *testing.T) {
	db := openTestDB(t)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("ping mysql failed, host=%s, err=%v; if Navicat can connect but this test cannot, check the security group/MySQL whitelist for the runtime environment's egress IP", extractHost(testDSN()), err)
	}

	t.Logf("successfully connected to mysql, host=%s", extractHost(testDSN()))
}

func TestQuery(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	model := NewBlockModel(db)
	block, err := model.FindOneBySlot(ctx, 23)
	if err != nil {
		t.Fatalf("find block by slot failed: %v", err)
	}
	t.Logf("found block: %+v", block)

}

func TestOperator(t *testing.T) {
	db := openTestDB(t)
	model := NewBlockModel(db)
	ctx := context.Background()

	t.Run("create and query", func(t *testing.T) {
		block := newTestBlock()
		cleanupBlockBySlot(t, db, block.Slot)

		if err := model.Insert(ctx, block); err != nil {
			t.Fatalf("insert block failed: %v", err)
		}
		//测试结束前执行
		// t.Cleanup(func() {
		// 	cleanupBlockByID(t, db, block.Id)
		// })

		if block.Id == 0 {
			t.Fatalf("expected auto-increment id after insert")
		}

		gotByID, err := model.FindOne(ctx, block.Id)
		if err != nil {
			t.Fatalf("find by id failed: %v", err)
		}
		assertBlockEqual(t, gotByID, block)

		gotBySlot, err := model.FindOneBySlot(ctx, block.Slot)
		if err != nil {
			t.Fatalf("find by slot failed: %v", err)
		}
		assertBlockEqual(t, gotBySlot, block)
	})

	t.Run("update", func(t *testing.T) {
		block := newTestBlock()
		cleanupBlockBySlot(t, db, block.Slot)

		if err := model.Insert(ctx, block); err != nil {
			t.Fatalf("insert block failed: %v", err)
		}
		t.Cleanup(func() {
			cleanupBlockByID(t, db, block.Id)
		})
		block.Status = 2
		block.SolPrice = 321.123456789
		block.ErrMessage = "update in test"

		if err := model.Update(ctx, block); err != nil {
			t.Fatalf("update block failed: %v", err)
		}

		got, err := model.FindOne(ctx, block.Id)
		if err != nil {
			t.Fatalf("query updated block failed: %v", err)
		}

		if got.Status != block.Status {
			t.Fatalf("unexpected status after update: got=%d want=%d", got.Status, block.Status)
		}
		if got.ErrMessage != block.ErrMessage {
			t.Fatalf("unexpected err_message after update: got=%q want=%q", got.ErrMessage, block.ErrMessage)
		}
		if got.SolPrice != block.SolPrice {
			t.Fatalf("unexpected sol_price after update: got=%v want=%v", got.SolPrice, block.SolPrice)
		}
	})

	t.Run("delete", func(t *testing.T) {
		block := newTestBlock()
		cleanupBlockBySlot(t, db, block.Slot)

		if err := model.Insert(ctx, block); err != nil {
			t.Fatalf("insert block failed: %v", err)
		}

		if err := model.Delete(ctx, block.Id); err != nil {
			t.Fatalf("delete block failed: %v", err)
		}

		_, err := model.FindOne(ctx, block.Id)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("expected record not found after delete, got err=%v", err)
		}
	})

	t.Run("transaction commit", func(t *testing.T) {
		block := newTestBlock()
		cleanupBlockBySlot(t, db, block.Slot)

		err := db.Transaction(func(tx *gorm.DB) error {
			txModel := NewBlockModel(tx)
			if err := txModel.Insert(ctx, block); err != nil {
				return err
			}

			block.Status = 2
			block.ErrMessage = "committed"
			return txModel.Update(ctx, block)
		})
		if err != nil {
			t.Fatalf("transaction commit failed: %v", err)
		}
		// t.Cleanup(func() {
		// 	cleanupBlockByID(t, db, block.Id)
		// })

		got, err := model.FindOne(ctx, block.Id)
		if err != nil {
			t.Fatalf("query committed block failed: %v", err)
		}
		if got.Status != 2 || got.ErrMessage != "committed" {
			t.Fatalf("unexpected committed block: status=%d err_message=%q", got.Status, got.ErrMessage)
		}
	})

	t.Run("transaction rollback", func(t *testing.T) {
		block := newTestBlock()
		cleanupBlockBySlot(t, db, block.Slot)

		expectedErr := errors.New("rollback for test")
		err := db.Transaction(func(tx *gorm.DB) error {
			txModel := NewBlockModel(tx)
			if err := txModel.Insert(ctx, block); err != nil {
				return err
			}
			return expectedErr
		})

		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected rollback error %v, got %v", expectedErr, err)
		}

		_, findErr := model.FindOneBySlot(ctx, block.Slot)
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			t.Fatalf("expected record not found after rollback, got err=%v", findErr)
		}
	})

	t.Run("duplicate slot", func(t *testing.T) {
		block := newTestBlock()
		cleanupBlockBySlot(t, db, block.Slot)

		if err := model.Insert(ctx, block); err != nil {
			t.Fatalf("insert first block failed: %v", err)
		}
		t.Cleanup(func() {
			cleanupBlockByID(t, db, block.Id)
		})

		duplicate := newTestBlock()
		duplicate.Slot = block.Slot
		duplicate.BlockHeight = block.BlockHeight + 1

		err := model.Insert(ctx, duplicate)
		if err == nil {
			t.Fatalf("expected duplicate slot insert to fail")
		}
	})
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(mysql.Open(testDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open mysql with gorm failed, dsn=%s, err=%v", maskDSN(testDSN()), err)
	}

	return db
}

func testDSN() string {
	if dsn := os.Getenv("TEST_MYSQL_DSN"); dsn != "" {
		return dsn
	}
	return defaultTestDSN
}

func newTestBlock() *Block {
	now := time.Now().UTC().Truncate(time.Second)
	seed := time.Now().UnixNano()

	return &Block{
		Slot:        seed,
		BlockHeight: seed,
		BlockTime:   now,
		Status:      1,
		SolPrice:    123.456789,
		ErrMessage:  fmt.Sprintf("test-block-%d", seed),
	}
}

func cleanupBlockByID(t *testing.T, db *gorm.DB, id int64) {
	t.Helper()

	if id == 0 {
		return
	}

	if err := db.Where("id = ?", id).Delete(&Block{}).Error; err != nil {
		t.Fatalf("cleanup block by id failed: %v", err)
	}
}

func cleanupBlockBySlot(t *testing.T, db *gorm.DB, slot int64) {
	t.Helper()

	if err := db.Where("slot = ?", slot).Delete(&Block{}).Error; err != nil {
		t.Fatalf("cleanup block by slot failed: %v", err)
	}
}

func assertBlockEqual(t *testing.T, got, want *Block) {
	t.Helper()

	if got.Id != want.Id {
		t.Fatalf("unexpected id: got=%d want=%d", got.Id, want.Id)
	}
	if got.Slot != want.Slot {
		t.Fatalf("unexpected slot: got=%d want=%d", got.Slot, want.Slot)
	}
	if got.BlockHeight != want.BlockHeight {
		t.Fatalf("unexpected block_height: got=%d want=%d", got.BlockHeight, want.BlockHeight)
	}
	if !got.BlockTime.Equal(want.BlockTime) {
		t.Fatalf("unexpected block_time: got=%s want=%s", got.BlockTime, want.BlockTime)
	}
	if got.Status != want.Status {
		t.Fatalf("unexpected status: got=%d want=%d", got.Status, want.Status)
	}
	if got.SolPrice != want.SolPrice {
		t.Fatalf("unexpected sol_price: got=%v want=%v", got.SolPrice, want.SolPrice)
	}
	if got.ErrMessage != want.ErrMessage {
		t.Fatalf("unexpected err_message: got=%q want=%q", got.ErrMessage, want.ErrMessage)
	}
}

func extractHost(dsn string) string {
	start := strings.Index(dsn, "@tcp(")
	if start == -1 {
		return "unknown"
	}
	start += len("@tcp(")

	end := strings.Index(dsn[start:], ")")
	if end == -1 {
		return "unknown"
	}

	return dsn[start : start+end]
}

func maskDSN(dsn string) string {
	at := strings.Index(dsn, "@")
	if at == -1 {
		return dsn
	}

	colon := strings.Index(dsn, ":")
	if colon == -1 || colon > at {
		return dsn
	}

	return dsn[:colon+1] + "******" + dsn[at:]
}
