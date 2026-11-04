// Command chartsample writes Samples/excel-charts/Charts.xlsx: a workbook with a chart
// of each kind ExcelLite draws, made through the model and the .xlsx
// writer, for checking them against Excel; with -created,
// Samples/excel-charts/Charts-Created.xlsx: charts made and changed as the Insert
// menu and the Chart Design tab do.
//
//	go run ./excel/cmd/chartsample [-o Samples/excel-charts/Charts.xlsx]
//	go run ./excel/cmd/chartsample -created [-o Samples/excel-charts/Charts-Created.xlsx]
package main

import (
	"flag"
	"log"

	"officelite/excel/fileio"
	"officelite/excel/internal/chartsample"
)

func main() {
	out := flag.String("o", "", "output file (Samples/excel-charts/Charts.xlsx, or Samples/excel-charts/Charts-Created.xlsx with -created)")
	created := flag.Bool("created", false, "write the charts made through the editing commands")
	flag.Parse()
	wb := chartsample.Workbook()
	name := "Samples/excel-charts/Charts.xlsx"
	if *created {
		wb, name = chartsample.Created(), "Samples/excel-charts/Charts-Created.xlsx"
	}
	if *out != "" {
		name = *out
	}
	if err := fileio.SaveFile(wb, name, fileio.XLSX, 0); err != nil {
		log.Fatal(err)
	}
}
