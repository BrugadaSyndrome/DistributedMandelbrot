# Distributed Mandelbrot

## CLI Options
I tried to make all parameters have a reasonable default but supplying these parameters should give you plenty of
options when messing around with generating images.

The notation for the Coordinator and Worker sections below is:
name (type: default value) - description

### Coordinator
* isCoordinator (boolean: false) - Set this to true to run this instance as the Coordinator
* boundary (integer: 4.0) - The escape boundary that the mandel process should bail out at
* centerX (integer: -0.5) - The center X point to generate the image from
* centerY (integer: 0) - The center Y point to generate the image from
* height (integer: 1080) - The height of the resulting image(s)
* magnificationEnd (float: 1.5) - The zoom level to end generating images at
* magnificationStart (float: 0.5) - The zoom level to start generating images at
* magnificationStep (float: 1.0) - The amount to zoom between each generated image
* maxIterations (integer: 1000) - The iteration count that the mandel process should bail out at
* paletteFile (string: '') - Specify a json file with a list of RGBA values to be used as the color palette like so:
 [ {"R": 1, "G": 0, "B": 0, "A": 255}, {"R": 0, "G": 1, "B": 0, "A": 255} ]
 * smoothColoring (boolean: false) - Enable smooth coloring technique
 * width (integer: 1920) - The width of the resulting image

### Worker
* isWorker (boolean: false) - Set this to true to run this instance as a worker pool
* coordinatorAddress (string: 'localhost:10000') - The ip address the workers should use to communicate with the coordinator
* WorkerCount (integer: 2) - Set the number of worker threads to spin up