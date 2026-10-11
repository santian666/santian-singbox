function updateGroupLayout() {
  var groups = document.getElementById("groups");
  if (!groups) {
    return;
  }
  var count = groups.querySelectorAll(".group-card").length;
  groups.classList.toggle("single-row", count > 0 && count < 9);
}

var observer = new MutationObserver(updateGroupLayout);

function startGroupLayout() {
  var groups = document.getElementById("groups");
  if (!groups) {
    requestAnimationFrame(startGroupLayout);
    return;
  }
  observer.observe(groups, { childList: true });
  updateGroupLayout();
}

startGroupLayout();
