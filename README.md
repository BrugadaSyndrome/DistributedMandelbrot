# Distributed Mandelbrot

This program is able to generate Mandelbrot images in a distributed manner. It leverages RPC calls to allow the worker
instance to be on the same or different hardware as the coordinator instance.

### CLI Options

To keep things simple the number of cli options are limited to these settings.

* mode - Set this to 'coordinator' or 'worker' to specify what mode you want the program instance to run in.
* settings - Set this to the name of the json file with the settings you want to use. The coordinator and the worker
  modes have different options that can be specified in the json file. These options are explained in further detail
  below.
* workers (int: 2) - The number of workers that will be created to process tasks from the coordinator when the mode is
  set to 'worker'

View the run_coordinator.cmd and run_worker.cmd files to see examples.

### Coordinator Mode Settings

When the program runs in coordinator mode, the program will generate tasks for the workers to do and then use the
results from the workers to generate the final image(s). **An instance of this mode should be up and running before
another instance is started in worker mode.**

View the coordinator/settings.go file to see what options can be passed in and what their default values are. Also view
the settings_coordinator.json file to see an example set of run settings.

### Worker Mode Settings

When the program is run in worker mode, it processes the tasks that are given it by the coordinator. **An instance of
this mode should not be run until another instance in coordinator mode is running already**

View the worker/settings.go file to see what options can be passed in and what their default values are. Also view the
settings_worker.json file to see an example set of run settings.