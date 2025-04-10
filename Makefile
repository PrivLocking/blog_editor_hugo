ifeq ($(USER),root)
$(info )
$(info 838388381 you can NOT run by root !!! )
$(info )
$(error )
endif

#cpuN:=$(shell echo -n $$((`cat /proc/cpuinfo  |grep processor |wc -l` - 1 )))
ifndef cpuN
cpuN:=$(shell cat /proc/cpuinfo  |grep processor |wc -l |sed -e s,^,-1\ +\ ,g |xargs -l expr)
endif


#debianVer1:=$(shell cat /etc/os-release  |grep VERSION_ID |cut -d\" -f 2)
debianVer1:=$(shell cat /etc/os-release  |grep VERSION_ID |cut -d\" -f 2|cut -d= -f 2|tr -d .)
ifeq (3213,$(debianVer1))
debianVer1:=3212
endif



nice:= chrt --idle 0 nice -n 19

extA:= -fPIC           -fPIC -ffunction-sections -fdata-sections -Os -flto -Wl,--gc-sections -Wl,--strip-all 
extA:= -fPIC -fPIC -O3       -ffunction-sections -fdata-sections -Wl,--gc-sections -Wl,--strip-all -Wl,-rpath,/home/nginX/LD_LIBRARY_PATH_3212
extA:= -fPIC       -O3       -ffunction-sections -fdata-sections -Wl,--gc-sections -Wl,--strip-all -Wl,-rpath,/home/nginX/LD_LIBRARY_PATH_3212


build_dir:=src
dst_path:=$(shell realpath /home/nginX/Blog_editor_$(debianVer1))


libApath:=/home/nginX/Zlib_1.3.1_3212/lib/libz.a
#libApath:=/usr/lib/libz.a





define dispText

aaa ### need network
aaa:    $(aaa)

bbb ### no network is needed
bbb:    $(bbb)

m:      $(m)
cb:     $(cb)   $($(cb))
b  		$(b)


dst_path:	$(dst_path)
build_dir:	$(build_dir)


endef
export dispText

m:=vim Makefile
aaa:=clean config help make_build make_install 
aaa:=clean config help make_build 
aaa:=clean config help make_build      build_static     make_install
aaa:=clean config help make_build                       make_install

bbb:=clean conf2 conf3 help make_build make_install

display:
	echo "$${dispText}"
	@ls -l --color src/*.go bin/*.bin
	@echo


m:
	$(m)

aaa: $(aaa)

bbb: $(bbb)

v1: src/blog_editor.go 
	vim $<
v2: src/hugo_update_daemon.go 
	vim $<


	

cb:=clear_dst_bin
$(cb):= chmod -R u+w $(dst_path) ; rm -fr $(dst_path)/* ; mkdir $(dst_path)/11 ;rmdir $(dst_path)/11 ;
cb $(cb):
	$($(cb))

b:=build make make_build
b $(b):
	@echo; echo ==$@
	rm -f bin/blog_editor.bin
	rm -f bin/hugo_update_daemon.bin
	CGO_ENABLED=0  $(nice) go build -ldflags="-extldflags=-static" \
		-o bin/blog_editor.bin \
		src/blog_editor.go \
		> log.goatcounter.04.make_build.go.txt
	CGO_ENABLED=0  $(nice) go build -ldflags="-extldflags=-static" \
		-o bin/hugo_update_daemon.bin \
		src/hugo_update_daemon.go \
		> log.goatcounter.05.make_build.go.txt
	@ls -l -d --color bin/*.bin 
	@echo




i mi in install make_install:  cb
	@echo; echo ==$@
	mkdir -p $(dst_path)
	@chmod -R u+w		$(dst_path)
	rm -fr $(dst_path)/*
	mkdir -p $(dst_path)/bin
	cp bin/blog_editor.bin         $(dst_path)/bin/
	cp bin/hugo_update_daemon.bin  $(dst_path)/bin/
	-strip        	$(dst_path)/bin/*.bin
	chmod -R a-w	$(dst_path)
	@ls -l --color 	$(dst_path)/bin/*.bin
	@file          	$(dst_path)/bin/*.bin
	@echo


