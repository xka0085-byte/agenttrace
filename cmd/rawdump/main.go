package main
import ("database/sql";"fmt";"os";_ "modernc.org/sqlite")
func main() {
db,_:=sql.Open("sqlite",os.Getenv("TEMP")+"\\agenttrace_demo.db")
defer db.Close()
var tc,sc int
db.QueryRow("SELECT COUNT(*) FROM traces").Scan(&tc)
db.QueryRow("SELECT COUNT(*) FROM spans").Scan(&sc)
fmt.Printf("DB: %d traces, %d spans\n",tc,sc)
rows,_:=db.Query("SELECT id,trace_id,name FROM spans LIMIT 10")
for rows.Next(){var i,t,n string;rows.Scan(&i,&t,&n);fmt.Printf("  %s -> %s: %s\n",i,t,n)}
}