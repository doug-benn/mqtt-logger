package main

func main() {

	//ctx, cancel := context.WithCancel(context.Background())
	//defer cancel()
	// db, err := database.NewDatabase(ctx)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer db.Stop()
	//rainfallRepo := repository.NewDatabaseService(db)
	// fmt.Println(rainfallRepo)
	// rainfallRepo.Migrate(ctx)

	runMQTT()

}
