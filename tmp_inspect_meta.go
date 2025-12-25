package main
import(
 "fmt"
 "log"
 "github.com/nir0k/GeoRAW/internal/media"
)
func main(){
 files:=[]string{"tests/100CANON/IMG_3279.JPG","tests/100CANON/IMG_3278.CR3","tests/100CANON/IMG_3277.CR3","tests/100CANON/IMG_3276.CR3"}
 for _,f:=range files{
  m,err:=media.ReadSeriesMetadata(f)
  if err!=nil{log.Fatalf("%s: %v",f,err)}
  fmt.Printf("%s hdr=%v focus=%v seq=%d exp=%f f=%.1f iso=%d time=%s\n",f,m.HDRHint,m.FocusBr,seq(f),m.ExposureTime,m.FNumber,m.ISO,m.CaptureTime.Format("15:04:05.000"))
 }
}
func seq(path string) int{
 s:=path
 for len(s)>0{
  c:=s[len(s)-1]
  if c<'0'||c>'9'{s=s[:len(s)-1];continue}
  break
 }
 i:=len(s)-1
 for i>=0&&s[i]>='0'&&s[i]<='9'{i--}
 if i==len(s)-1{return -1}
 var n int
 fmt.Sscanf(s[i+1:],"%d",&n)
 return n
}
