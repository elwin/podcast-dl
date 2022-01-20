package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gosimple/slug"
	"github.com/korovkin/limiter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var rootCmd = &cobra.Command{
	Use:   "podcast-dl <url>",
	Short: "podcast-dl allows you to download videos from a podcast / RSS feed.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]
		return run(url)
	},
}

var (
	logger      *zap.Logger
	concurrency = 10
)

func main() {
	var err error
	logger, err = zap.NewDevelopment()
	if err != nil {
		log.Fatal(err)
	}

	rootCmd.PersistentFlags().IntVarP(
		&concurrency,
		"concurrency",
		"c",
		concurrency,
		"Set level of concurrency",
	)

	if err := rootCmd.Execute(); err != nil {
		logger.Fatal(err.Error())
	}
}

type Item struct {
	Text        string `xml:",chardata"`
	Guid        string `xml:"guid"`
	Title       string `xml:"title"`
	Description struct {
		Text string `xml:",chardata"`
		P    []struct {
			Text string `xml:",chardata"`
			Img  struct {
				Text string `xml:",chardata"`
				Src  string `xml:"src,attr"`
			} `xml:"img"`
		} `xml:"p"`
	} `xml:"description"`
	Author    string `xml:"author"`
	PubDate   string `xml:"pubDate"`
	Enclosure struct {
		Text string `xml:",chardata"`
		Type string `xml:"type,attr"`
		URL  string `xml:"url,attr"`
	} `xml:"enclosure"`
	Subtitle string `xml:"subtitle"`
	Summary  struct {
		Text string `xml:",chardata"`
		P    []struct {
			Text string `xml:",chardata"`
			Img  struct {
				Text string `xml:",chardata"`
				Src  string `xml:"src,attr"`
			} `xml:"img"`
		} `xml:"p"`
	} `xml:"summary"`
	Explicit string `xml:"explicit"`
}

type Channel struct {
	Text        string `xml:",chardata"`
	Title       string `xml:"title"`
	Description struct {
		Text string `xml:",chardata"`
		A    struct {
			Text string `xml:",chardata"`
			Href string `xml:"href,attr"`
		} `xml:"a"`
	} `xml:"description"`
	Link struct {
		Text string `xml:",chardata"`
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
		Type string `xml:"type,attr"`
	} `xml:"link"`
	LastBuildDate string `xml:"lastBuildDate"`
	Subtitle      string `xml:"subtitle"`
	Summary       struct {
		Text string `xml:",chardata"`
		A    struct {
			Text string `xml:",chardata"`
			Href string `xml:"href,attr"`
		} `xml:"a"`
	} `xml:"summary"`
	Explicit string `xml:"explicit"`
	Item     []Item `xml:"item"`
}

type Rss struct {
	XMLName xml.Name `xml:"rss"`
	Text    string   `xml:",chardata"`
	Atom    string   `xml:"atom,attr"`
	Content string   `xml:"content,attr"`
	Itunes  string   `xml:"itunes,attr"`
	Version string   `xml:"version,attr"`
	Channel Channel  `xml:"channel"`
}

func run(url string) error {
	client := resty.New()

	resp, err := client.R().Get(url)
	if err != nil {
		return err
	}

	body := resp.Body()
	var rss Rss
	if err := xml.Unmarshal(body, &rss); err != nil {
		return err
	}

	limit := limiter.NewConcurrencyLimiter(concurrency)
	for _, item := range rss.Channel.Item {
		item := item
		limit.Execute(func() {
			if err := download(item, client); err != nil {
				log.Fatal(err)
			}
		})
	}
	limit.Wait()

	return nil
}

func download(item Item, client *resty.Client) error {
	logger.Info("downloading file", zap.String("pubDate", item.PubDate))

	if item.Enclosure.Type != "video/mp4" {
		return nil
	}

	videoUrl := item.Enclosure.URL
	publishedDate, err := time.Parse("2006-01-02T15:04Z", item.PubDate)
	if err != nil {
		return err
	}

	title := fmt.Sprintf(
		"%s/%s.mp4",
		slug.Make(item.Title),
		publishedDate.Format("2006-01-02-15-04"),
	)


	_, err = client.R().SetOutput(title).Get(videoUrl)
	if err != nil {
		return err
	}

	logger.Info("downloaded file", zap.String("pubDate", item.PubDate))

	return nil
}