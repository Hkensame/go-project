package initialize

import (
	"fmt"
	"log"
	"os"
	"time"

	gb "goods_srv/global"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// 这个函数必须在InitConfig和InitLog后调用
func InitDB() {
	MysqlConfig := gb.ServerConfig.MysqlConfig
	dsn := fmt.Sprintf("%s:%s@%s(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		MysqlConfig.UserName,
		MysqlConfig.Password,
		MysqlConfig.NetType,
		MysqlConfig.Host,
		MysqlConfig.Port,
		MysqlConfig.DBName,
	)
	log := logger.New(
		log.New(os.Stdout, "", log.LstdFlags|log.Llongfile),
		logger.Config{
			SlowThreshold: 1 * time.Second,
			Colorful:      true,
			LogLevel:      logger.Info,
		},
	)
	var err error
	gb.DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: false,
		},
		Logger:         log,
		TranslateError: true,
	})
	if err != nil {
		zap.S().Errorw("DB创建失败", "msg", err.Error())
	}
}
