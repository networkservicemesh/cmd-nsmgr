# Cmd NsMgr build scenarious.

 1. Only docker
    
    1.1 `docker build .` - perform build inside docker, no local SDK references are allowed.
        Inside: nsm build
    
    1.2 `docker run $(docker build -q . --target test) - perform execution of tests.
        Inside: nsm build
                nsm test (will check if inside docker will just run all tests found in /bin/*.test)
    
    1.3 `docker build . --build-arg BUILD=false` - build container, but copy binaries from local build ./dist folder.
        - require local compile of nsmgr with `nsm build` or
        Inside:
            docker copy 
        
    1.4 `docker run $(docker build -q . --target test --build-arg BUILD=false)` - perform execution of tests, copy test binaries from local host.
        Inside:
            docker copy
            nsm test (will check if inside docker will just run all tests found in /bin/*.test)
                will start spire server and run all tests
    1.5 Debug container inside docker
    
 2. Using nsm cli tool.
    
    2.1 `nsm build`  - just build all stuff and docker conatiner
    
    2.2 `nsm test` - perform a build and run tests inside docker.
        
        2.2.1 Debug of tests
            `nsm test --debug`, will run tests with dlv to debug contaniner
           
        2.2.2 Debug of selectd test
            `nsm test --debug --test nsmgr-test.test` - will run debug only for one package, will filter other packages.

               