package main

import "github.com/GreedyKomodoDragon/redis-operator/internal/backup"

func main() {
	// Start the backup service
	backup.Run()
}
