package main
import ("fmt";"log";"net/http";"os";"github.com/stockyard-dev/stockyard-trough/internal/server";"github.com/stockyard-dev/stockyard-trough/internal/store")
func main(){port:=os.Getenv("PORT");if port==""{port="8670"};dataDir:=os.Getenv("DATA_DIR");if dataDir==""{dataDir="./trough-data"}
db,err:=store.Open(dataDir);if err!=nil{log.Fatalf("trough: %v",err)};defer db.Close();srv:=server.New(db)
fmt.Printf("\n  Trough — LLM API cost monitor\n  Dashboard:  http://localhost:%s/ui\n  API:        http://localhost:%s/api\n\n",port,port)
log.Printf("trough: listening on :%s",port);log.Fatal(http.ListenAndServe(":"+port,srv))}
