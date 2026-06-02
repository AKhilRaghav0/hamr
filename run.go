// runStdio starts the server with stdio transport.
func (s *Server) runStdio() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Set up config file watcher if configFile is specified.
	if s.config.configFile != "" {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("failed to create config file watcher: %w", err)
		}
		defer watcher.Close()

		if err := watcher.Add(s.config.configFile); err != nil {
			return fmt.Errorf("failed to watch config file %s: %w", s.config.configFile, err)
		}

		// Channel to receive watcher errors.
		watcherErrors := make(chan error)
		// Channel to signal the watcher goroutine to stop.
		watcherDone := make(chan struct{})
		// Goroutine to handle file events and reload config.
		go func() {
			defer close(watcherErrors)
			for {
				select {
				case <-watcherDone:
					return
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}
					if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
						// Reload config.
						if err := s.reloadConfigLocked(); err != nil {
							s.logger.Error("failed to reload config", "error", err)
						} else {
							s.logger.Info("config reloaded due to file change")
						}
					}
				case err, ok := <-watcherErrors:
					if !ok {
						return
					}
					s.logger.Error("config watcher error", "error", err)
				}
			}
		}()
		// Ensure the watcher goroutine stops when the context is done.
		go func() {
			<-ctx.Done()
			close(watcherDone)
		}()
	}

	handler := &mcpHandler{server: s}
	t := transport.NewStdio(handler)

	s.logger.Info("starting mcpx server", "name", s.name, "version", s.version, "transport", "stdio")
	return t.Run(ctx)
}