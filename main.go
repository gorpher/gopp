package main

import (
	"bufio"
	"bytes"
	"container/list"
	"context"
	"flag"
	"fmt"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
	"github.com/pkg/browser"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

type TreeNode struct {
	Name     string      `json:"name,omitempty"`
	Value    int         `json:"value"`
	Path     string      `json:"path"`
	Children []*TreeNode `json:"children,omitempty"`
}

func ShortName(name string) string {
	longP, short := path.Split(name)
	if len(short) <= 2 {
		short = path.Base(strings.TrimRight(longP, "/"))
	}
	return short
}

// GenTreeNodeByDependency TODO There is a better algorithm that needs to be optimized
func GenTreeNodeByDependency(startNode string, allNodes map[string]*TreeNode) []*TreeNode {
	//total := len(nodes)
	queue := list.New()
	rootNode := allNodes[startNode]
	visitedNode := make(map[string]*TreeNode, 0)
	for _, node := range rootNode.Children {
		nNode := &TreeNode{
			Name:     node.Path,
			Path:     node.Path,
			Value:    node.Value,
			Children: make([]*TreeNode, 0),
		}
		if len(node.Children) == 0 {
			nNode.Name = ShortName(nNode.Path)
			visitedNode[node.Path] = nNode
			continue
		}
		queue.PushBack(nNode)
	}
	for queue.Len() > 0 {
		node, ok := queue.Remove(queue.Front()).(*TreeNode)
		if !ok {
			break
		}
		treeNode, ok := allNodes[node.Path]
		if !ok {
			continue
		}
		node.Name = ShortName(node.Path)
		node.Value += len(treeNode.Children)
		visitedNode[node.Path] = node
		for _, v := range treeNode.Children {
			_, ok = visitedNode[v.Path]
			if len(v.Children) > 0 {
				node.Value += len(v.Children)
				visitedNode[node.Path] = node
			}
			if !ok {
				nNode := &TreeNode{Name: ShortName(node.Path), Path: v.Path, Value: v.Value}
				visitedNode[nNode.Path] = nNode
				queue.PushBack(nNode)
			}
		}
	}
	nodes := make([]*TreeNode, len(visitedNode))
	var i int
	for _, node := range visitedNode {
		nodes[i] = node
		i += 1
	}
	return nodes
}

var ToolTipFormatter = `
function (info) {
      let formatUtil = echarts.format;
      var value = info.value;
      var treePathInfo = info.treePathInfo;
      var treePath = [];
      for (var i = 1; i < treePathInfo.length; i++) {
        treePath.push(treePathInfo[i].name);
      }
      return '<div style="background-color:#fafafa;color:#000;margin:0px;padding:3px">'+[
        '<div class="tooltip-title">' +
          formatUtil.encodeHTML(info.data.path) +
          '</div>',
        'Have: ' + formatUtil.addCommas(value) + ' Dependencies'
      ].join('')+'</div>';
    }
`

// AddSeries adds new data sets.
func AddSeries(c *charts.TreeMap, name string, data []*TreeNode, options ...charts.SeriesOpts) *charts.TreeMap {
	series := charts.SingleSeries{Name: name, Type: types.ChartTreeMap, Data: data}
	series.ConfigureSeriesOpts(options...)
	c.MultiSeries = append(c.MultiSeries, series)
	return c
}
func ShowEcharts(treeMap []*TreeNode) *charts.TreeMap {
	graph := charts.NewTreeMap()
	graph.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeMacarons}),
		charts.WithTitleOpts(opts.Title{
			Title: "Golang Packages Dependency",
			Left:  "center",
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show:      true,
			Formatter: opts.FuncOpts(ToolTipFormatter),
		}),
		charts.WithToolboxOpts(opts.Toolbox{
			Show:   true,
			Orient: "horizontal",
			Left:   "right",
			Feature: &opts.ToolBoxFeature{
				SaveAsImage: &opts.ToolBoxFeatureSaveAsImage{
					Show: true, Title: "Save as image"},
				Restore: &opts.ToolBoxFeatureRestore{
					Show: true, Title: "Reset"},
			}}),
	)
	// Add initialized data to graph.
	AddSeries(graph, "", treeMap).
		SetSeriesOptions(
			charts.WithTreeMapOpts(
				opts.TreeMapChart{
					Animation:  true,
					Roam:       true,
					UpperLabel: &opts.UpperLabel{Show: true},
					Levels: &[]opts.TreeMapLevel{
						{ // Series
							ItemStyle: &opts.ItemStyle{
								BorderColor: "#f0fdf4",
								BorderWidth: 1,
								GapWidth:    1},
							UpperLabel: &opts.UpperLabel{Show: false},
						},
						{ // Level
							ItemStyle: &opts.ItemStyle{
								BorderColor: "#166534",
								BorderWidth: 2,
								GapWidth:    1},
							Emphasis: &opts.Emphasis{
								ItemStyle: &opts.ItemStyle{BorderColor: "#16a34a"},
							},
						},
						{ // Node
							ColorSaturation: []float32{0.35, 0.5},
							ItemStyle: &opts.ItemStyle{
								GapWidth:              1,
								BorderWidth:           0,
								BorderColorSaturation: 0.6,
							},
						},
					},
				},
			),
			charts.WithItemStyleOpts(opts.ItemStyle{BorderColor: "#fff"}),
			charts.WithLabelOpts(opts.Label{Show: true, Position: "inside", Color: "White"}),
		)
	return graph
}

func getGraphBytes(ctx context.Context) (data []byte) {
	cCtx, cancelFunc := context.WithTimeout(ctx, time.Second*2)
	go func() {
		var scanner = bufio.NewScanner(os.Stdin)
		for {
			select {
			case <-cCtx.Done():
				return
			default:
				if scanner.Scan() {
					data = append(data, scanner.Bytes()...)
				}
			}
		}
	}()
	defer cancelFunc()
	<-cCtx.Done()
	var err error
	if len(data) < 1 {
		cmd := exec.Command("go", "mod", "graph")
		if data, err = cmd.CombinedOutput(); nil != err {
			log.Fatalf("go mod graph cmd run failed: %+v", err)
		} else {
			return data
		}
	}
	return data
}

func main() {
	o := flag.String("o", "", "output file name.")
	addr := flag.String("addr", ":18888", "http server address.")
	flag.Parse()
	inputFile := flag.Arg(flag.NArg() - 1)
	var (
		byteData []byte
		err      error
	)
	if inputFile != "" {
		byteData, err = os.ReadFile(inputFile)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		byteData = getGraphBytes(context.Background())
	}
	var startNode string
	strBuffer := bytes.NewBuffer(byteData)
	nodes := make(map[string]*TreeNode, 0)
	for {
		line, err := strBuffer.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		parts := strings.Split(string(line), " ")
		headName := strings.Trim(parts[0], " \t\r\n")
		tailName := strings.Trim(parts[1], " \t\r\n")
		headName = strings.Split(headName, "@")[0]
		tailName = strings.Split(tailName, "@")[0]
		// Skip not third packages
		if !strings.ContainsAny(headName, "/") {
			continue
		}
		if !strings.ContainsAny(tailName, "/") {
			continue
		}
		if startNode == "" {
			startNode = headName
		}
		var (
			headNode *TreeNode
			tailNode *TreeNode
			ok       bool
		)
		headNode, ok = nodes[headName]
		if !ok {
			headNode = &TreeNode{
				Name:     headName,
				Path:     headName,
				Value:    1,
				Children: make([]*TreeNode, 0),
			}
			nodes[headName] = headNode
		}
		tailNode, ok = nodes[tailName]
		if !ok {
			tailNode = &TreeNode{
				Name:     tailName,
				Path:     tailName,
				Value:    1,
				Children: make([]*TreeNode, 0),
			}
			nodes[tailName] = tailNode
		}
		var exist bool
		for _, v := range headNode.Children {
			if v.Name == tailNode.Name {
				exist = true
				break
			}
		}
		if !exist {
			headNode.Children = append(headNode.Children, tailNode)
		}
		nodes[headName] = headNode
	}
	trees := GenTreeNodeByDependency(startNode, nodes)
	echarts := ShowEcharts(trees)
	if *o != "" {
		var f *os.File
		f, err = os.OpenFile(*o, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 700)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close() //nolint
		err = echarts.Render(f)
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		err = echarts.Render(w)
		if err != nil {
			log.Fatal(err)
		}
	})
	address := strings.SplitN(*addr, ":", 2)
	if len(address) == 2 {
		if address[0] == "" {
			address[0] = "127.0.0.1"
		}
		err = browser.OpenURL("http://"+address[0] + ":" + address[1])
		if err != nil {
			panic(err)
		}
	}
	fmt.Printf("Listening on Addr %s\n", *addr)
	if err = http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
		return
	}
}
