'use client';
import Indexing from '../Indexing';
import {useProject} from '../ProjectShell';
export default function IndexingPage(){const {project,locale}=useProject();return <Indexing key={project.id} projectId={project.id} locale={locale}/>}
