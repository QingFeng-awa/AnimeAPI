// Package wallet 货币系统
package wallet

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/FloatTech/floatbox/file"
	sql "github.com/FloatTech/sqlite"
)

// Storage 货币系统
type Storage struct {
	sync.RWMutex
	db sql.Sqlite
}

// Wallet 钱包
type Wallet struct {
	UID   int64
	Money int
}

// SubsidyRecord 补贴记录
type SubsidyRecord struct {
	Time  string
	UID   int64
	Money int
}

var (
	sdb = &Storage{
		db: sql.New("data/wallet/wallet.db"),
	}
	walletName         = "Atri币"
	publicFundsAccount = int64(0)
)

func init() {
	if file.IsNotExist("data/wallet") {
		err := os.MkdirAll("data/wallet", 0755)
		if err != nil {
			panic(err)
		}
	}
	err := sdb.db.Open(time.Hour * 24)
	if err != nil {
		panic(err)
	}
	err = sdb.db.Create("storage", &Wallet{})
	if err != nil {
		panic(err)
	}
	err = sdb.db.Create("subsidy", &SubsidyRecord{})
	if err != nil {
		panic(err)
	}
}

// GetWalletName 获取货币名称
func GetWalletName() string {
	return walletName
}

// SetWalletName 设置货币名称
func SetWalletName(name string) {
	walletName = name
}

// GetWalletOf 获取钱包数据
func GetWalletOf(uid int64) (money int) {
	return sdb.getWalletOf(uid).Money
}

// GetGroupWalletOf 获取多人钱包数据
//
// if sort == true,由高到低排序; if sort == false,由低到高排序
func GetGroupWalletOf(sortable bool, uids ...int64) (wallets []Wallet, err error) {
	return sdb.getGroupWalletOf(sortable, uids...)
}

// InsertWalletOf 更新钱包数据
//
// money > 0 增加,money < 0 减少
func InsertWalletOf(uid int64, money int) error {
	sdb.Lock()
	defer sdb.Unlock()
	lastMoney := sdb.getWalletOf(uid)
	newMoney := lastMoney.Money + money
	if newMoney < 0 {
		newMoney = 0
	}
	return sdb.updateWalletOf(uid, newMoney)
}

// GetPublicFundsId 获取公款账户 ID
//
// 一般公款账户 ID 为 0
func GetPublicFundsAccountId() int64 {
	return publicFundsAccount
}

// GetPublicFundsWallet 获取公款账户余额
func GetPublicFundsWallet() int {
	return sdb.getWalletOf(publicFundsAccount).Money
}

// InsertPublicFundsWallet 更新公款账户余额
//
// 与 InsertWalletOf 行为一致，只是将目标锁定为了公款账户。
func InsertPublicFundsWallet(money int) error {
	return InsertWalletOf(publicFundsAccount, money)
}

// 获取钱包数据 (no lock)
//
// WARNING: 谨防数据库并发问题。
func (s *Storage) getWalletOf(uid int64) (wallet Wallet) {
	uidstr := strconv.FormatInt(uid, 10)
	_ = s.db.Find("storage", &wallet, "WHERE uid = ?", uidstr)
	return
}

// 获取钱包数据组
func (s *Storage) getGroupWalletOf(sortable bool, uids ...int64) (wallets []Wallet, err error) {
	s.RLock()
	defer s.RUnlock()
	wallets = make([]Wallet, 0, len(uids))
	sort := "ASC"
	if sortable {
		sort = "DESC"
	}
	info := Wallet{}
	q, sl := sql.QuerySet("WHERE uid", "IN", uids)
	err = s.db.FindFor("storage", &info, q+" ORDER BY money "+sort, func() error {
		wallets = append(wallets, info)
		return nil
	}, sl...)
	return
}

// 更新钱包 (no lock)
//
// WARNING: 谨防数据库并发问题。
func (s *Storage) updateWalletOf(uid int64, money int) (err error) {
	return s.db.Insert("storage", &Wallet{
		UID:   uid,
		Money: money,
	})
}

// IssuancePovertySubsidies 发放贫困补助
//
// 从公款账户扣款向目标发放补贴，补贴记录会被写入数据库。
func IssuancePovertySubsidies(uid int64, money int) error {
	if money <= 0 {
		return fmt.Errorf("补贴金额不能为负数")
	}
	if GetPublicFundsWallet() < money {
		return fmt.Errorf("公款账户余额不足")
	}

	sdb.Lock()
	defer sdb.Unlock()

	publicFundsWallet := sdb.getWalletOf(publicFundsAccount).Money
	if publicFundsWallet < money {
		return fmt.Errorf("公款账户余额不足")
	}

	err := sdb.updateWalletOf(publicFundsAccount, publicFundsWallet-money)
	if err != nil {
		return err
	}

	userWallet := sdb.getWalletOf(uid)
	newUserMoney := userWallet.Money + money
	if newUserMoney < 0 {
		newUserMoney = 0
	}

	err = sdb.updateWalletOf(uid, newUserMoney)
	if err != nil {
		return err
	}

	now := time.Now()
	dateStr := now.Format("2006-01-02")
	err = sdb.db.Insert("subsidy", &SubsidyRecord{
		Time:  dateStr,
		UID:   uid,
		Money: money,
	})
	if err != nil {
		return err
	}

	return nil
}

// GetFirstSubsidyRecord 获取第一个补贴记录
//
// 没有记录返回 nil
func GetFirstSubsidyRecord(uid int64) *SubsidyRecord {
	sdb.RLock()
	defer sdb.RUnlock()

	var record SubsidyRecord
	uidstr := strconv.FormatInt(uid, 10)
	err := sdb.db.Find("subsidy", &record, "WHERE uid = ? ORDER BY rowid ASC LIMIT 1", uidstr)
	if err != nil {
		return nil
	}

	return &record
}
