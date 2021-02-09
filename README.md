# CA Vaccine Alerts

This program generates tweets for [@CaVaccine](https://twitter.com/CaVaccine) (or any authenticated user) with
information on vaccine sites with availability in CA.

It currently does this by querying the lat long of every zip, which could probably be reduced to limit API calls.

To run, simply run `go run main.go` with the following environment variables set:

```
API_KEY
API_SECRET
ACCESS_TOKEN
ACCESS_SECRET
```

Issues / Pull requests welcome. 
