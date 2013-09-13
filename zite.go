package main

import (
    "fmt"
    "github.com/russross/blackfriday"
    "code.google.com/p/gcfg"
    "text/template"
    "io/ioutil"
    "path/filepath"
    "os"
    "io"
    "strings"
    "regexp"
    "bytes"
)

var config struct {
    Src string
    Out string
}



type Element struct {
    Parent *Element
    Path string
    Filename string
    Type string
    URL string

    // Page
    Title string
    Content string

    // Directory
    Children []*Element
}


func replaceIncludes(path string, content string) string {
    includeRegexp, err := regexp.Compile(`\{\{include ([^}]+)}}`)
    if err != nil {
        panic(err)
    }
    baseDir := filepath.Dir(path)

    fn := func(include string) string {
        includePath := include[10:len(include)-2]
        relativePath := filepath.Join(baseDir, includePath)
        buffer, err := ioutil.ReadFile(relativePath)
        if err != nil {
            panic(err)
        }
        return string(buffer)
    }
    result := includeRegexp.ReplaceAllStringFunc(content, fn)
    return result
}

func (e *Element) String() string {
    switch e.Type {
    case "page":
        return fmt.Sprintf("(page: %s %s)", e.Filename, e.Title)
    case "dir":
        childrenS := []string{}
        for _, child := range e.Children {
            childrenS = append(childrenS, child.String())
        }
        return fmt.Sprintf("(dir: %s [%s])", e.Filename, strings.Join(childrenS, ", "))
    default:
        return fmt.Sprintf("(file: %s)", e.Filename)
    }
}

func markdown(s string) string {
    return string(blackfriday.MarkdownBasic([]byte(s)))
}

func readTextFile(path string) (string, error) {
    buffer, err := ioutil.ReadFile(path)
    if err != nil {
        return "", err
    }
    return string(buffer), nil
}

func pathToURL(path string) string {
    return ensureExt(path, ".html")[len(config.Src):]
}

func readFile(path string) (*Element, error) {
    ext := filepath.Ext(path)
    if ext == ".md" {
        content, err := readTextFile(path)
        if err != nil {
            return nil, err
        }
        lines := strings.Split(content, "\n")
        return &Element {
            Path: path,
            URL: pathToURL(path),
            Filename: filepath.Base(path),
            Type: "page",
            Title: lines[0],
            Content: strings.Join(lines[2:], "\n"),
        }, nil
    } else if ext == ".html" {
        content, err := readTextFile(path)
        if err != nil {
            return nil, err
        }
        return &Element {
            Path: path,
            URL: pathToURL(path),
            Filename: filepath.Base(path),
            Type: "page",
            Content: replaceIncludes(path, content),
        }, nil
    } else {
        return &Element {
            Path: path,
            URL: pathToURL(path),
            Filename: filepath.Base(path),
            Type: "file",
        }, nil;
    }
}

func readDirectory(path string) (*Element, error) {
    dir := &Element{
        Path: path,
        Filename: filepath.Base(path),
        Type: "dir",
    };
    files, err := ioutil.ReadDir(path)
    if err != nil {
        return nil, err
    }
    for _, fi := range files {
        var e *Element
        if fi.Name()[0:1] == "_" {
            continue
        }
        if fi.IsDir() {
            e, err = readDirectory(filepath.Join(path, fi.Name()))
        } else {
            e, err = readFile(filepath.Join(path, fi.Name()))
        }
        if err != nil {
            return nil, err
        }
        e.Parent = dir
        dir.Children = append(dir.Children, e)
    }

    return dir, nil
}

func getTemplate(path string) (string, error) {
    fmt.Println("Finding template in", path)
    templateFilename := fmt.Sprintf("%s/_template.html", path)
    if _, err := os.Stat(templateFilename); err != nil {
        return getTemplate(filepath.Dir(path))
    }
    text, err := readTextFile(templateFilename)
    if err != nil {
        return "", err
    }
    return replaceIncludes(templateFilename, text), nil
}

var templateFuncs template.FuncMap = template.FuncMap {
    "markdown": markdown,
}

func renderFile(el *Element) (string, error) {
    path := el.Path
    ext := filepath.Ext(path)
    if ext == ".html" {
        tmpl := template.New("template")
        tmpl.Funcs(templateFuncs)
        tmpl, err := tmpl.Parse(el.Content)
        if err != nil {
            return "", err
        }
        var buffer bytes.Buffer
        err = tmpl.Execute(&buffer, el)
        if err != nil {
            return "", err
        }
        return buffer.String(), nil
    } else if ext == ".md" {
        templateText, err := getTemplate(filepath.Dir(path))
        if err != nil {
            return "", err
        }
        page, err := readFile(path)
        if err != nil {
            return "", err
        }
        tmpl := template.New("template")
        tmpl.Funcs(templateFuncs)
        tmpl, err = tmpl.Parse(templateText)
        if err != nil {
            return "", err
        }
        var buffer bytes.Buffer
        err = tmpl.Execute(&buffer, page)
        if err != nil {
            return "", err
        }
        return buffer.String(), nil
    } else {
        panic("Not sure how to render")
    }
}

func ensurePath(path string) error {
    dir := filepath.Dir(path)
    if _, err := os.Stat(dir); err != nil {
        err = os.MkdirAll(dir, 0755)
        if err != nil {
            return err
        }
    }
    return nil
}

func ensureExt(path, ext string) string {
    curExt := filepath.Ext(path)
    return strings.Replace(path, curExt, ext, 1)
}

func writeFile(path, content string) error {
    err := ensurePath(path)
    if err != nil {
        return err
    }
    return ioutil.WriteFile(path, []byte(content), 0600)
}

func copyFile(source, dest string) error {
    ensurePath(dest)
    fin, err := os.Open(source)
    if err != nil {
        return err
    }
    fout, err := os.Create(dest)
    if err != nil {
        return err
    }
    io.Copy(fout, fin)
    fin.Close()
    fout.Close()
    return nil
}

func stripPath(path string) string {
    return path[len(config.Src)+1:]
}

func generate(el *Element) error {
    switch el.Type {
    case "file":
        fmt.Println("Copying", el.Path)
        destPath := filepath.Join(config.Out, stripPath(el.Path))
        err := copyFile(el.Path, destPath)
        if err != nil {
            return err
        }
    case "page":
        fmt.Println("Rendering", el.Path)
        destPath := ensureExt(filepath.Join(config.Out, stripPath(el.Path)), ".html")
        rendered, err := renderFile(el)
        if err != nil {
            return err
        }
        err = writeFile(destPath, rendered)
        if err != nil {
            return err
        }
    case "dir":
        for _, el := range el.Children {
            err := generate(el)
            if err != nil {
                return err
            }
        }
    }
    return nil
}

func readConfig(path string) {
    config.Src = "src"
    config.Out = "www"

    configFile := fmt.Sprintf("%s/zite.ini", path)

    if _, err := os.Stat(configFile); err == nil {
        err = gcfg.ReadFileInto(&config, configFile)
        if err != nil {
            fmt.Println("Could not read config file", err);
            os.Exit(4)
        }
    }
}


func main() {
    readConfig(".")
    dirEl, err := readDirectory(config.Src)
    if err != nil {
        panic(err)
    }
    err = generate(dirEl)
    if err != nil {
        panic(err)
    }
}