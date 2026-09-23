import type { ReactElement } from "react";
import type { FieldError } from "../api/types";

export function RequestForm({
	title,
	description,
	fieldErrors,
	onTitleChange,
	onDescriptionChange,
	onSubmit,
	submitLabel,
	pending,
}: {
	title: string;
	description: string;
	fieldErrors: FieldError[];
	onTitleChange: (value: string) => void;
	onDescriptionChange: (value: string) => void;
	onSubmit: () => void;
	submitLabel: string;
	pending: boolean;
}): ReactElement {
	return (
		<form
			onSubmit={(event) => {
				event.preventDefault();
				onSubmit();
			}}
		>
			<label>
				Title
				<input
					value={title}
					onChange={(event) => onTitleChange(event.target.value)}
				/>
			</label>
			{fieldErrors
				.filter((error) => error.field === "title")
				.map((error) => (
					<p role="alert" key={`${error.field}-${error.code}-${error.message}`}>
						{error.message}
					</p>
				))}
			<label>
				Description
				<textarea
					value={description}
					onChange={(event) => onDescriptionChange(event.target.value)}
				/>
			</label>
			{fieldErrors
				.filter((error) => error.field === "description")
				.map((error) => (
					<p role="alert" key={`${error.field}-${error.code}-${error.message}`}>
						{error.message}
					</p>
				))}
			<button type="submit" disabled={pending}>
				{submitLabel}
			</button>
		</form>
	);
}
