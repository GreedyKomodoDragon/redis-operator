package main

import (
	"os"

	initbackup "github.com/GreedyKomodoDragon/redis-operator/internal/init-backup"
)

func main() {
	service := initbackup.NewService()
	if err := service.Run(); err != nil {
		os.Exit(1)
	}
}
