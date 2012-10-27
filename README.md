plist
=====

Quick and dirty implementation of Apple's binary plist format.

This was written to facilitate a command line app to edit shopping lists from the ShopShop iOS app (which stores its lists in binary1 plist format in a DropBox folder).


Todo
----

For general use it needs:

- Implement missing data types (e.g. Date)
- Handling of XML plist format
- Fix up error handling (add recover, double check we're catching everything)
- Support writing 1-byte object ids (try 1 byte, if it doesn't work, return an error and try again with 2 byte)
- Flesh out godocs
- Add Tests

Example Code
------------

``` go
package main

import (
  "github.com/dunhamsteve/plist"
  "fmt"
  "io/ioutil"
  "os"
)

func must(err error) {
  if err != nil {
    fmt.Println(err)
    os.Exit(-1)
  }
}

type Item struct {
  Done  bool
  Count string
  Name  string
}

type ShopFile struct {
  Color        []float64
  ShoppingList []Item
}

var list *ShopFile

func main() {
  fname := os.ExpandEnv("$HOME/Dropbox/ShopShop/Shopping List.shopshop")
  f, err := os.Open(fname)
  must(err)

  list := new(ShopFile)
  
  err = plist.Unmarshal(f, list)
  must(err)
  
  // Add Item
  list.ShoppingList = append(list.ShoppingList, Item{false, "", "new item"})
  
  // Save list
  out, err := plist.Marshal(list)
  must(err)
  err = ioutil.WriteFile(fname, out, 0644)
  must(err)
}
```
