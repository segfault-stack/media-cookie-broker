export function showPopupError(container, message) {
  const error = container.ownerDocument.createElement("div");
  error.className = "popup-error";
  error.setAttribute("role", "alert");
  error.textContent = message || "Could not open the recovery context.";
  container.replaceChildren(error);
}

export async function runRecoveryAction({button, resultElement, requestRecovery}) {
  resultElement.replaceChildren();
  button.disabled = true;
  button.textContent = "Opening recovery…";
  try {
    const response = await requestRecovery();
    if (!response?.ok) showPopupError(resultElement, response?.error);
    return response;
  } catch (error) {
    const response = {ok: false, error: String(error?.message || error)};
    showPopupError(resultElement, response.error);
    return response;
  } finally {
    button.textContent = "Refresh session";
    button.disabled = false;
  }
}
