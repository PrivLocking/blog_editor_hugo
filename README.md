# Blog Editor in Go-LANG

A stand alone editor for online edit your blog for Hugo static system, in GO-LANG.
it must work with hugo, and nginx/haproxy.

## it list on 3 parts:
*  blog_editor.go
*  hugo_update_daemon.go
*  blogedit.html

##  blog_editor.go
Use with a static html to edit and submit your hugo Markdown file.

User submit the file by this prog, no PHP is needed.


## hugo_update_daemon.go
After user submit the hugo Markdown file,

Use this daemon to call the hugo to update the blog/website.

Separate the prog, to prevent being hacked.


## Example html: `html/blogedit.html`
An example html for the user/client use to create/edit/submit the Markdown file.

It use `prism.css` and `marked.min.js`


## for more details :
Please check my [Blog](https://blog00.jjj123.com/post/2025/04/20250406_225415/).

