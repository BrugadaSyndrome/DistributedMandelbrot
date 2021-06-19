# Distributed Mandelbrot

This program is able to generate Mandelbrot images in a distributed manner. It leverages RPC calls to allow the worker
instance to be on the same or different hardware as the coordinator instance.

### CLI Options

To keep things simple the number of cli options are limited to these two settings.

* mode - Set this to 'coordinator' or 'worker' to specify what mode you want the program instance to run in.
* settings - Set this to the name of the json file with the settings you want to use. The coordinator and the worker
  modes have different options that can be specified in the json file. These options are explained in further detail
  below.

### Coordinator Mode Settings

When the program runs in coordinator mode, the program will generate tasks for the workers to do and then use the
results from the workers to generate the final image(s). **An instance of this mode should be up and running before
another instance is started in worker mode.**

The coordinator mode settings json file has the following settings you can specify:

* Boundary (integer: 100) - The escape boundary that the mandel process should bail out at
* EscapeColor (color.RGBA) - Specify the color to fill in the points in the Mandelbrot set
* GeneratePaletteSettings ([]GeneratePaletteSettings) - Specify list of GeneratePaletteSettings objects (see
  coordinator.go file) to be used to generate a color palette. This feature is useful when you want to use a large color
  palette and don't want to type it out yourself. Example:
  ```[ { "StartColor": {"R": 255, "G": 0, "B": 0, "A": 255}, "EndColor": {"R": 0, "G": 255, "B": 255, "A": 255}, "NumberColors": 50 } ]```
  **When this setting is specified, then the Palette option will be ignored.**
* Height (integer: 1080) - The height of the resulting image(s)
* MaxIterations (integer: 1000) - The iteration count that the mandel process should bail out at
* Palette ([]color.RGBA) - Specify list of color.RGBA objects to be used as the color palette like so:
  ```[ {"R": 255, "G": 0, "B": 0, "A": 255}, {"R": 0, "G": 255, "B": 0, "A": 255} , {"R": 0, "G": 0, "B": 255, "A": 255} ].```
  **When the GeneratePaletteSettings is specified, then this option will be ignored.**
* SmoothColoring (boolean: false) - Enable smooth coloring technique to blend between colors. **Must have more than one
  color in the palette to use this feature.**
* SuperSampling (int: 1) - The amount of super sampling (anti-aliasing) to use. Setting this to 1 means no AA.
  **Using this feature will increase each task workload exponentially.**
* TransitionSettings ([]TransitionSettings) - Specify a list of TransitionSettings objects (see coordinator.go file) to
  use in generating how the zooming in will be calculated. Example:
  ```[ {"StartX": -0.75, "StartY": 0, "EndX": -0.761574, "EndY": -0.0847596, "MagnificationStart": 1, "MagnificationEnd": 78125, "MagnificationStep": 2 } ]```
* Width (integer: 1920) - The width of the resulting image(s)

### Worker Mode Settings

When the program is run in worker mode, it processes the tasks that are given it by the coordinator. **An instance of
this mode should not be run until another instance in coordinator mode is running already**

The worker mode settings json file has the following settings you can specify:

* CoordinatorAddress (string: localhost) - The address the coordinator is using
* CoordinatorPort (int: 10000) - The port the coordinator is using
* WorkerCount (int: 2) - The number of worker threads to run.
* WorkerAddress (string: localhost) - The IP address the worker will use.