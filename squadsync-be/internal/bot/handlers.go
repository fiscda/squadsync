package bot

// registerAll registers every command, component and modal handler of the
// bot. Add new ones here; keep each feature's handlers in its own file.
func (r *Router) registerAll() {
	r.Command(r.csatornaCommand())
	r.Component(visszajelzesRoute, r.answerButton)
	r.Command(r.esemenyCommand())
	r.Modal(esemenyModalRoute, r.proposeFromModal)
	r.Component(veglegesitesRoute, r.confirmButton)
}
